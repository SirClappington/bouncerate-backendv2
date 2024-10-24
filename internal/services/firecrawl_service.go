package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
func (fc *FirecrawlClient) CrawlWebsite(website string, excludePaths []string, maxDepth int) (*firecrawl.CrawlResponse, error) {
	crawlParams := &firecrawl.CrawlParams{
		ExcludePaths: excludePaths,
		MaxDepth:     &maxDepth,
	}

	crawlResult, err := fc.Client.AsyncCrawlURL(website, crawlParams, nil)
	if err != nil {
		return nil, fmt.Errorf("crawl failed: %v", err)
	}
	return crawlResult, nil
}

func (fc *FirecrawlClient) GetCrawlStatus(crawlID string) (*firecrawl.CrawlStatusResponse, error) {
	status, err := fc.Client.CheckCrawlStatus(crawlID)
	if err != nil {
		return nil, fmt.Errorf("failed to get crawl status: %v", err)
	}
	return status, nil
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

// MapWebsite initiates a new map job for the given website.
func (fc *FirecrawlClient) MapWebsite(website string, limit *int) (*firecrawl.MapResponse, error) {
	params := &firecrawl.MapParams{
		Limit: limit,
	}

	mapResponse, err := fc.Client.MapURL(website, params)
	if err != nil {
		return nil, fmt.Errorf("failed to map website: %v", err)
	}

	if !mapResponse.Success {
		return nil, fmt.Errorf("map operation failed for %s: %s", website, mapResponse.Error)
	}

	if mapResponse.Links == nil {
		return nil, fmt.Errorf("received nil Links in response for %s", website)
	}

	return mapResponse, nil
}
