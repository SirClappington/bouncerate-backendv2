package services

import (
	"context"
	"fmt"
	"log"
)

type AnalysisService struct {
	firebase *FirebaseService
	logger   *log.Logger
}

func NewAnalysisService(firebase *FirebaseService, logger *log.Logger) *AnalysisService {
	return &AnalysisService{
		firebase: firebase,
		logger:   logger,
	}
}

func (as *AnalysisService) CalculateAveragePrice(ctx context.Context, locationName, category string) (float64, error) {
	// Retrieve location data from Firebase
	location, err := as.firebase.GetLocation(ctx, locationName)
	if err != nil {
		return 0, fmt.Errorf("error retrieving location data: %v", err)
	}

	// Calculate the average price for the given category
	var total float64
	var count int
	for _, competitor := range location.Competitors {
		for _, product := range competitor.Products {
			if product.Category == category {
				total += product.Price
				count++
			}
		}
	}

	if count == 0 {
		return 0, fmt.Errorf("no products found for category %s", category)
	}

	averagePrice := total / float64(count)
	return averagePrice, nil
}

func (as *AnalysisService) CalculateBreakEvenPoint(purchasePrice, averagePrice float64) (int, error) {
	if averagePrice == 0 {
		return 0, fmt.Errorf("average price cannot be zero")
	}

	breakEvenPoint := int(purchasePrice / averagePrice)
	return breakEvenPoint, nil
}
