package services

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"googlemaps.github.io/maps"
)

type CompetitorService struct {
	firecrawl *FirecrawlClient
	places    *maps.Client
	firebase  *FirebaseService
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

func NewCompetitorService(firecrawlKey, placesKey, firebaseCredentialsFile, firebaseBucketName string, logger *log.Logger) (*CompetitorService, error) {
	// Initialize Firecrawl
	firecrawlClient, err := NewFirecrawlClient(firecrawlKey)
	if err != nil {
		return nil, err
	}

	// Initialize Places Client
	placesClient, err := maps.NewClient(maps.WithAPIKey(placesKey))
	if err != nil {
		return nil, err
	}

	// Initialize Firebase Service
	firebaseService, err := NewFirebaseService(firebaseCredentialsFile, firebaseBucketName, logger)
	if err != nil {
		return nil, err
	}

	return &CompetitorService{
		firecrawl: firecrawlClient,
		places:    placesClient,
		firebase:  firebaseService,
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
		return nil, fmt.Errorf("error searching for competitors: %v", err)
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

	// Check for errors
	for err := range errs {
		s.logger.Printf("Error encountered: %v", err)
	}

	return &CompetitorSearchResult{
		Competitors: competitors,
		Location:    location,
		TotalFound:  len(competitors),
	}, nil
}

func (s *CompetitorService) processCompetitor(ctx context.Context, name, website string) (*Competitor, error) {
	// First try to map the website
	s.logger.Printf("Mapping website: %s", website)
	mapResponse, err := s.firecrawl.MapWebsite(ctx, website)
	if err != nil {
		s.logger.Printf("Error mapping website %s: %v", website, err)
		// Continue with crawl as fallback
	}

	var relevantURLs []string
	if mapResponse != nil && mapResponse.Links != nil {
		s.logger.Printf("Found %d links from mapping for website %s", len(mapResponse.Links), website)
		relevantURLs = filterRelevantURLs(mapResponse.Links)
	}

	if len(relevantURLs) == 0 {
		// If no relevant URLs found through mapping, try crawling
		s.logger.Printf("No relevant URLs found for website %s, falling back to crawl", website)
		crawlResponse, err := s.firecrawl.CrawlWebsite(ctx, website, nil, 500)
		if err != nil {
			s.logger.Printf("Error initiating crawl for website %s: %v", website, err)
			return nil, err
		}

		if crawlResponse != nil && crawlResponse.Success {
			crawlID := crawlResponse.ID
			s.logger.Printf("Crawl initiated for website %s with ID %s", website, crawlID)
			statusResponse, err := s.firecrawl.GetCrawlStatus(ctx, crawlID)
			if err != nil {
				s.logger.Printf("Error checking crawl status for crawl ID %s: %v", crawlID, err)
				return nil, err
			}

			// Collect links from each FirecrawlDocument
			for _, doc := range statusResponse.Data {
				relevantURLs = append(relevantURLs, doc.Links...)
			}
			s.logger.Printf("Crawl completed for website %s, found %d links", website, len(relevantURLs))
			relevantURLs = filterRelevantURLs(relevantURLs)
		}
	}

	// Extract product information from relevant pages
	var products []Product
	for _, url := range relevantURLs {
		s.logger.Printf("Extracting products from URL: %s", url)
		extractedProducts, err := s.firecrawl.ScrapeWebsite(ctx, url)
		if err != nil {
			s.logger.Printf("Error extracting products from %s: %v", url, err)
			continue // Skip failed extractions
		}
		products = append(products, extractedProducts)
	}

	if len(products) == 0 {
		s.logger.Printf("No products found for website %s", website)
		return nil, nil // Skip if no products found
	}

	s.logger.Printf("Found %d products for website %s", len(products), website)
	return &Competitor{
		Name:     name,
		Website:  website,
		Products: products,
	}, nil
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
