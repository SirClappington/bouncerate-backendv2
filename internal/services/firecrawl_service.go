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
	"sync"
	"time"

	"github.com/mendableai/firecrawl-go"
)

// FireCrawlClient manages interactions with the FireCrawl API.
type FirecrawlClient struct {
	apiKey  string
	baseURL string
	Version string
	Client  *firecrawl.FirecrawlApp
	limiter *RateLimiter
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

type RateLimiter struct {
	tokens        int
	maxTokens     int
	tokenInterval time.Duration
	mu            sync.Mutex
}

func NewRateLimiter(maxTokens int, tokenInterval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		tokens:        maxTokens,
		maxTokens:     maxTokens,
		tokenInterval: tokenInterval,
	}

	go rl.refillTokens()

	return rl
}

func (rl *RateLimiter) refillTokens() {
	ticker := time.NewTicker(rl.tokenInterval)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		if rl.tokens < rl.maxTokens {
			rl.tokens++
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.tokens > 0 {
		rl.tokens--
		return true
	}

	return false
}

// NewFireCrawlClient creates a new instance of FireCrawlClient.
func NewFirecrawlClient(apiKey string) (*FirecrawlClient, error) {
	return &FirecrawlClient{
		apiKey:  apiKey,
		baseURL: "https://api.firecrawl.dev/",
		limiter: NewRateLimiter(5, time.Second), // 5 requests per second
	}, nil
}

// CrawlWebsite initiates a new crawl job for the given website.
func (fc *FirecrawlClient) CrawlWebsite(ctx context.Context, website string, options interface{}, limit int) (*firecrawl.CrawlResponse, error) {
	if !fc.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	url := fmt.Sprintf("%scrawl", fc.baseURL)
	requestBody := map[string]interface{}{
		"website": website,
		"limit":   limit,
		"options": options,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fc.apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
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

func (fc *FirecrawlClient) GetCrawlStatus(ctx context.Context, crawlID string) (*firecrawl.CrawlStatusResponse, error) {
	if !fc.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	url := fmt.Sprintf("%sstatus?crawl_id=%s", fc.baseURL, crawlID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+fc.apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
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
	if !fc.limiter.Allow() {
		return Product{}, fmt.Errorf("rate limit exceeded")
	}

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
		return Product{}, fmt.Errorf("failed to execute request: %v", err)
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
func (fc *FirecrawlClient) MapWebsite(ctx context.Context, website string) (*MapResponse, error) {
	if !fc.limiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	resp, err := fc.Client.MapURL(website, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to map website: %v", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("failed to map website: %s", resp.Error)
	}

	return &MapResponse{
		Success: resp.Success,
		Error:   resp.Error,
		Links:   resp.Links,
	}, nil
}
