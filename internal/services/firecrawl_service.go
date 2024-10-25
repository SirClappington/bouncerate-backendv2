package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/mendableai/firecrawl-go"
)

// FireCrawlClient manages interactions with the FireCrawl API.
type FirecrawlClient struct {
	apiKey  string
	baseURL string
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

type CrawlResponse struct {
	// Define the fields based on the expected response
}

type StatusResponse struct {
	// Define the fields based on the expected response
}

// NewFireCrawlClient creates a new instance of FireCrawlClient.
func NewFirecrawlClient(apiKey string) (*FirecrawlClient, error) {
	return &FirecrawlClient{
		apiKey:  apiKey,
		baseURL: "https://api.firecrawl.dev/v1",
	}, nil
}

// CrawlWebsite initiates a new crawl job for the given website.
func (fc *FirecrawlClient) CrawlWebsite(website string, options interface{}, limit int) (*firecrawl.CrawlResponse, error) {
	url := fmt.Sprintf("%s/crawl?website=%s&limit=%d", fc.baseURL, website, limit)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+fc.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to crawl website: %s", string(body))
	}

	var crawlResponse CrawlResponse
	if err := json.Unmarshal(body, &crawlResponse); err != nil {
		return nil, fmt.Errorf("failed to parse crawl response: %v", err)
	}

	return &firecrawl.CrawlResponse{}, nil
}

func (fc *FirecrawlClient) GetCrawlStatus(crawlID string) (*firecrawl.CrawlStatusResponse, error) {
	url := fmt.Sprintf("%s/status?crawl_id=%s", fc.baseURL, crawlID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+fc.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get crawl status: %s", string(body))
	}

	var statusResponse StatusResponse
	if err := json.Unmarshal(body, &statusResponse); err != nil {
		return nil, fmt.Errorf("failed to parse status response: %v", err)
	}

	return &firecrawl.CrawlStatusResponse{}, nil
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

	req, err := http.NewRequestWithContext(ctx, "POST", fc.baseURL+"scrape", bytes.NewBuffer(jsonBody))
	if err != nil {
		return Product{}, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fc.apiKey)

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
func (fc *FirecrawlClient) MapWebsite(website string, limit *int) (*MapResponse, error) {
	url := fmt.Sprintf("%s/map?website=%s&limit=%d", fc.baseURL, website, *limit)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+fc.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to map website: %s", string(body))
	}

	var mapResponse MapResponse
	if err := json.Unmarshal(body, &mapResponse); err != nil {
		return nil, fmt.Errorf("failed to parse map response: %v", err)
	}

	return &mapResponse, nil
}
