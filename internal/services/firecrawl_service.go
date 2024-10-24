package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/mendableai/firecrawl-go"
)

// FireCrawlClient manages interactions with the FireCrawl API.
type FirecrawlClient struct {
	APIKey  string
	BaseURL string
	Version string
	Client  *firecrawl.FirecrawlApp
}

type MapParams struct {
	Limit             *int  `json:"limit,omitempty"`
	IncludeSubdomains *bool `json:"includeSubdomains,omitempty"`
	Search            *bool `json:"search,omitempty"`
	IgnoreSitemap     *bool `json:"ignoreSitemap,omitempty"`
}

type MapResponse struct {
	Success bool     `json:"success"`
	Error   string   `json:"error,omitempty"`
	Links   []string `json:"links,omitempty"`
}

type ExtractPrompt struct {
	ExtractPrompt string
}

// NewFireCrawlClient creates a new instance of FireCrawlClient.
func NewFirecrawlClient(apiKey string) (*FirecrawlClient, error) {
	baseURL := "https://api.firecrawl.dev/v1/"

	// Initialize FirecrawlApp
	app, err := firecrawl.NewFirecrawlApp(apiKey, baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize FirecrawlApp: %v", err)
	}

	client := &FirecrawlClient{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Client:  app,
	}

	return client, nil
}

// CrawlWebsite initiates a new crawl job for the given website.
func (fc *FirecrawlClient) CrawlWebsite(website string, excludePaths []string, maxDepth int) (*firecrawl.CrawlStatusResponse, error) {
	crawlParams := &firecrawl.CrawlParams{
		ExcludePaths: excludePaths,
		MaxDepth:     &maxDepth,
	}

	crawlResult, err := fc.Client.CrawlURL(website, crawlParams, nil)
	if err != nil {
		return nil, fmt.Errorf("crawl failed: %v", err)
	}
	return crawlResult, nil
}

func (fc *FirecrawlClient) ScrapeWebsite(ctx context.Context, productURL string) (Product, error) {
	extractSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name":  map[string]interface{}{"type": "string"},
			"price": map[string]interface{}{"type": "string"},
			"url":   map[string]interface{}{"type": "string"},
		},
		"required": []string{"name", "price", "url"},
	}

	extractPrompt := "Extract the main product from the page, including name and price. If a price range is given, only include the lowest price. Return the url of the page as well. Return the data as a JSON object with \"name\", \"price\", and \"url\" fields."

	scrapeParams := &firecrawl.ScrapeParams{
		Formats: []string{"extract"},
		Headers: &map[string]string{
			"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
		},
	}

	requestBody := map[string]interface{}{
		"url":     productURL,
		"formats": scrapeParams.Formats,
		"headers": scrapeParams.Headers,
		"extract": map[string]interface{}{
			"schema": extractSchema,
			"prompt": extractPrompt,
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return Product{}, fmt.Errorf("failed to marshal request body: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fc.BaseURL+"scrape", bytes.NewBuffer(jsonBody))
	if err != nil {
		return Product{}, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fc.APIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return Product{}, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Product{}, fmt.Errorf("failed to read response: %v", err)
	}

	var result struct {
		Data struct {
			Extract string `json:"extract"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return Product{}, fmt.Errorf("failed to parse response: %v", err)
	}

	var extractedProduct map[string]interface{}
	if err := json.Unmarshal([]byte(result.Data.Extract), &extractedProduct); err != nil {
		return Product{}, fmt.Errorf("failed to unmarshal extracted data: %v", err)
	}

	price, err := strconv.ParseFloat(extractedProduct["price"].(string), 64)
	if err != nil {
		return Product{}, fmt.Errorf("failed to parse price for product %s: %v", extractedProduct["name"], err)
	}

	product := Product{
		Name:  extractedProduct["name"].(string),
		Price: price,
		URL:   extractedProduct["url"].(string),
	}

	return product, nil
}

// Helper function to create a pointer to a bool
func BoolPtr(b bool) *bool {
	return &b
}

// MapWebsite initiates a new map job for the given website.
func (fc *FirecrawlClient) MapWebsite(website string, limit *int) (*MapResponse, error) {
	url := "https://api.firecrawl.dev/v1/map"

	// Prepare the payload dynamically to include the URL and optional limit
	payloadMap := map[string]interface{}{
		"url": website,
	}
	if limit != nil {
		payloadMap["limit"] = *limit
	}

	payload, err := json.Marshal(payloadMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request payload: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}

	// Retrieve the API key from environment variables
	apiKey := os.Getenv("FIRECRAWL_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("API key is missing: please set the FIRECRAWL_API_KEY environment variable")
	}

	// Set headers
	req.Header.Add("Authorization", "Bearer "+apiKey)
	req.Header.Add("Content-Type", "application/json")

	// Execute the request
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute HTTP request: %v", err)
	}
	defer res.Body.Close()

	// Check for non-200 status codes and handle errors
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("received non-200 status code: %d, response: %s", res.StatusCode, string(body))
	}

	// Check if the response body is empty or nil
	if res.Body == nil {
		return nil, fmt.Errorf("received empty response from Firecrawl API for %s", website)
	}

	// Parse the response body
	var mapResponse MapResponse
	err = json.NewDecoder(res.Body).Decode(&mapResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Firecrawl API response: %v", err)
	}

	// Check if the mapping operation was successful
	if !mapResponse.Success {
		return nil, fmt.Errorf("map operation failed for %s: %s", website, mapResponse.Error)
	}

	// Verify that `Links` is not nil or empty to avoid potential nil dereference errors
	if mapResponse.Links == nil {
		return nil, fmt.Errorf("received nil Links in response for %s", website)
	}

	return &mapResponse, nil
}
