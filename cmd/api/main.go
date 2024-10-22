package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/yourusername/bouncerate-backend/internal/errors"
	"github.com/yourusername/bouncerate-backend/internal/services"
)

var (
	competitorService *services.CompetitorService
	logger            *log.Logger
)

func init() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("No .env file found: %v", err)
	}

	// Initialize logger
	logger = log.New(os.Stdout, "[BOUNCERATE] ", log.LstdFlags)

	// Initialize services
	var err error
	competitorService, err = services.NewCompetitorService(
		os.Getenv("FIRECRAWL_API_KEY"),
		os.Getenv("GOOGLE_PLACES_API_KEY"),
		logger,
	)
	if err != nil {
		log.Fatalf("Failed to initialize competitor service: %v", err)
	}
}

func handleError(c *gin.Context, err error) {
	if apiErr, ok := err.(*errors.APIError); ok {
		switch apiErr.Type {
		case errors.ErrorTypeValidation:
			c.JSON(http.StatusBadRequest, apiErr)
		case errors.ErrorTypeNotFound:
			c.JSON(http.StatusNotFound, apiErr)
		case errors.ErrorTypeExternal:
			c.JSON(http.StatusServiceUnavailable, apiErr)
		case errors.ErrorTypeUnauthorized:
			c.JSON(http.StatusUnauthorized, apiErr)
		default:
			c.JSON(http.StatusInternalServerError, apiErr)
		}
		return
	}

	// Handle unknown errors
	c.JSON(http.StatusInternalServerError, errors.NewInternalError(err))
}

func searchCompetitors(c *gin.Context) {
	location := c.Query("location")
	if location == "" {
		handleError(c, errors.NewValidationError("location is required"))
		return
	}

	result, err := competitorService.SearchCompetitors(c.Request.Context(), location)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// Handler for getting competitor analysis
func getCompetitorAnalysis(c *gin.Context) {
	location := c.Query("location")
	category := c.Query("category")

	if location == "" || category == "" {
		c.JSON(400, gin.H{"error": "location and category are required"})
		return
	}

	// TODO: Implement competitor analysis
	c.JSON(200, gin.H{
		"message":  "Successfully analyzed competitors",
		"location": location,
		"category": category,
	})
}

// Handler for purchase analysis
func analyzePurchase(c *gin.Context) {
	var request struct {
		ProductType   string  `json:"productType" binding:"required"`
		PurchasePrice float64 `json:"purchasePrice" binding:"required"`
		Location      string  `json:"location" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// TODO: Implement purchase analysis
	c.JSON(200, gin.H{
		"message": "Successfully analyzed purchase",
		"request": request,
	})
}
