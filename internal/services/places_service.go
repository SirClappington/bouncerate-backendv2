package services

import (
	"context"
	"fmt"
	"time"

	"googlemaps.github.io/maps"
)

type PlacesClient struct {
	Client *maps.Client
}

type CompetitorResult struct {
	Name    string
	Website string
	PlaceID string
}

func NewPlacesClient(apiKey string) (*PlacesClient, error) {
	client, err := maps.NewClient(maps.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Maps client: %v", err)
	}
	return &PlacesClient{Client: client}, nil
}

func (pc *PlacesClient) SearchCompetitors(ctx context.Context, location string) ([]CompetitorResult, error) {
	var results []CompetitorResult
	err := retry(func() error {
		r, err := pc.Client.TextSearch(ctx, &maps.TextSearchRequest{
			Query: "Bounce house rentals in " + location,
		})
		if err != nil {
			return err
		}
		for _, place := range r.Results {
			details, err := pc.GetPlaceDetails(ctx, place.PlaceID)
			if err != nil {
				continue // Skip this competitor if we can't get details
			}
			results = append(results, CompetitorResult{
				Name:    place.Name,
				Website: details.Website,
				PlaceID: place.PlaceID,
			})
		}
		return nil
	})
	return results, err
}

func (pc *PlacesClient) GetPlaceDetails(ctx context.Context, placeID string) (*maps.PlaceDetailsResult, error) {
	var result *maps.PlaceDetailsResult
	err := retry(func() error {
		r, err := pc.Client.PlaceDetails(ctx, &maps.PlaceDetailsRequest{
			PlaceID: placeID,
			Fields:  []maps.PlaceDetailsFieldMask{maps.PlaceDetailsFieldMaskWebsite},
		})
		if err != nil {
			return err
		}
		result = &r
		return nil
	})
	return result, err
}

func retry(operation func() error) error {
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		err := operation()
		if err == nil {
			return nil
		}
		if i == maxRetries-1 {
			return err
		}
		time.Sleep(time.Duration(i+1) * time.Second)
	}
	return fmt.Errorf("operation failed after %d retries", maxRetries)
}
