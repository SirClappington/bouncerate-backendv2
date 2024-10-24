package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/mendableai/firecrawl-go"
	"googlemaps.github.io/maps"
)

type CompetitorService struct {
	firecrawl *firecrawl.FirecrawlApp
	places    *maps.Client
	logger    *log.Logger
}

type CompetitorSearchResult struct {
	Competitors []Competitor `json:"competitors"`
	Location    string       `json:"location"`
	TotalFound  int          `json:"totalFound"`
}

type Competitor struct {
	Name     string    `json:"name"`
	Website  string    `json:"website"`
	Products []Product `json:"products"`
}

type Product struct {
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	URL      string  `json:"url"`
	Category string  `json:"category"`
}

type ProductSchema struct {
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	URL      string  `json:"url,omitempty"`
	Category string  `json:"category"`
}

type ExtractSchema struct {
	Products []ProductSchema `json:"products"`
}

func NewCompetitorService(firecrawlKey, placesKey string, logger *log.Logger) (*CompetitorService, error) {
	// Initialize Firecrawl
	app, err := firecrawl.NewFirecrawlApp(
		firecrawlKey,
		"https://api.firecrawl.dev",
	)
	if err != nil {
		return nil, err
	}

	// Initialize Places Client
	placesClient, err := maps.NewClient(maps.WithAPIKey(placesKey))
	if err != nil {
		return nil, err
	}

	return &CompetitorService{
		firecrawl: app,
		places:    placesClient,
		logger:    logger,
	}, nil
}

func (s *CompetitorService) SearchCompetitors(ctx context.Context, location string) (*CompetitorSearchResult, error) {
	// Search for bounce house rental businesses in the area
	searchRequest := &maps.TextSearchRequest{
		Query: "bounce house rentals in " + location,
		Type:  "business",
	}

	response, err := s.places.TextSearch(ctx, searchRequest)
	if err != nil {
		return nil, err
	}

	// Process competitors concurrently with rate limiting
	var wg sync.WaitGroup
	results := make(chan Competitor, len(response.Results))
	errs := make(chan error, len(response.Results))
	semaphore := make(chan struct{}, 5) // Limit concurrent requests

	for _, place := range response.Results {
		wg.Add(1)
		go func(place maps.PlacesSearchResult) {
			defer wg.Done()

			semaphore <- struct{}{}        // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore

			// Get place details to get website
			detailsReq := &maps.PlaceDetailsRequest{
				PlaceID: place.PlaceID,
				Fields:  []maps.PlaceDetailsFieldMask{maps.PlaceDetailsFieldMaskWebsite},
			}

			details, err := s.places.PlaceDetails(ctx, detailsReq)
			if err != nil {
				s.logger.Printf("Error getting place details for %s: %v", place.Name, err)
				errs <- err
				return
			}

			if details.Website == "" {
				return // Skip places without websites
			}

			competitor, err := s.processCompetitor(ctx, place.Name, details.Website)
			if err != nil {
				s.logger.Printf("Error processing competitor %s: %v", place.Name, err)
				errs <- err
				return
			}
			if competitor != nil {
				results <- *competitor
			}
		}(place)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(results)
		close(errs)
	}()

	// Collect results
	var competitors []Competitor
	for competitor := range results {
		competitors = append(competitors, competitor)
	}

	return &CompetitorSearchResult{
		Competitors: competitors,
		Location:    location,
		TotalFound:  len(competitors),
	}, nil
}

func (s *CompetitorService) processCompetitor(ctx context.Context, name, website string) (*Competitor, error) {
	// First try to map the website
	mapResult, err := s.firecrawl.MapURL(website, &firecrawl.MapParams{
		IncludeSubdomains: BoolPtr(true),
		Limit:             IntPtr(500),
	})
	if err != nil {
		s.logger.Printf("Error mapping website %s: %v", website, err)
		// Continue with crawl as fallback
	}

	var relevantURLs []string
	if mapResult != nil && mapResult.Links != nil {
		relevantURLs = filterRelevantURLs(mapResult.Links)
	}

	// If no relevant URLs found through mapping, try crawling
	if len(relevantURLs) == 0 {
		s.logger.Printf("No relevant URLs found for website %s, falling back to crawl", website)
		crawlResult, err := s.firecrawl.AsyncCrawlURL(website, nil)
		if err != nil {
			s.logger.Printf("Error initiating crawl for website %s: %v", website, err)
			return nil, err
		}
		
		if crawlResult != nil && crawlResult.Success == true {
			crawlID := crawlResult.ID
			s.logger.Printf("Crawl initiated for website %s with ID %s", website, crawlID)
			result, err := s.firecrawl.CheckCrawlStatus(crawlID)
			if err != nil {
				s.logger.Printf("Error checking crawl status for crawl ID %s: %v", crawlID, err)
				return nil, err
		} else {
			s.logger.Printf("Crawl completed for website %s, found %d links", website, len(result.Data.Links))
			relevantURLs = filterRelevantURLs(result.Data.Links)
		}	
	}

	// Extract product information from relevant pages
	var products []Product
	for _, url := range relevantURLs {
		extractedProducts, err := s.extractProducts(url)
		if err != nil {
			s.logger.Printf("Error extracting products from %s: %v", url, err)
			continue // Skip failed extractions
		}
		products = append(products, extractedProducts...)
	}

	if len(products) == 0 {
		return nil, nil // Skip if no products found
	}

	return &Competitor{
		Name:     name,
		Website:  website,
		Products: products,
	}, nil
}

func (s *CompetitorService) extractProducts(url string) ([]Product, error) {
    // Create a schema that matches our struct
    schema := ExtractSchema{}
    schemaJSON, err := json.Marshal(schema)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal schema: %w", err)
    }

    // Set up the scrape parameters
    scrapeParams := &firecrawl.ScrapeParams{
        Formats: []string{"extract"},
        Extract: firecrawl.ExtractConfig{
            Schema: json.RawMessage(schemaJSON),
            Prompt: "Extract all rental products from the page. For each product, include its name, rental price, and URL if available. If a price range is given, use the lowest price. Categorize each product as either 'Bounce House', 'Water Slide', 'Obstacle Course', or 'Other'.",
        },
    }

    // Perform the scrape with extraction
    result, err := s.firecrawl.ScrapeURL(url, scrapeParams)
    if err != nil {
        return nil, fmt.Errorf("scrape failed: %w", err)
    }

    // Parse the extracted data
    var extractedData struct {
        Extract ExtractSchema `json:"extract"`
    }

    if err := json.Unmarshal(result.Data, &extractedData); err != nil {
        return nil, fmt.Errorf("failed to parse extracted data: %w", err)
    }

    // Convert to our Product type
    var products []Product
    for _, p := range extractedData.Extract.Products {
        products = append(products, Product{
            Name:     p.Name,
            Price:    p.Price,
            URL:      p.URL,
            Category: p.Category,
        })
    }

    return products, nil
}

func filterRelevantURLs(urls []string) []string {
	var relevant []string
	keywords := []string{
		"/products", "/rentals", "/inventory",
		"/bounce-house", "/inflatables",
		"/catalog", "/equipment", "/items",
	}

	for _, url := range urls {
		for _, keyword := range keywords {
			if strings.Contains(strings.ToLower(url), keyword) {
				relevant = append(relevant, url)
				break
			}
		}
	}
	return relevant
}

func BoolPtr(b bool) *bool {
	return &b
}

func IntPtr(i int) *int {
	return &i
}
