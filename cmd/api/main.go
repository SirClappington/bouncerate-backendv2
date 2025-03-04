package main

import (
	"log"
	"net/http"
	"os"

	"github.com/SirClappington/bouncerate-backendv2/internal/errors"
	"github.com/SirClappington/bouncerate-backendv2/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

var (
	competitorService *services.CompetitorService
	firebaseService   *services.FirebaseService
	analysisService   *services.AnalysisService
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
		os.Getenv("FIRECRAWL_BASE_URL"),
		os.Getenv("GOOGLE_PLACES_API_KEY"),
		os.Getenv("FIREBASE_CREDENTIALS_FILE"),
		os.Getenv("FIREBASE_BUCKET_NAME"),
		logger,
	)
	if err != nil {
		log.Fatalf("Failed to initialize competitor service: %v", err)
	}

	firebaseService, err = services.NewFirebaseService(
		os.Getenv("FIREBASE_CREDENTIALS_FILE"),
		os.Getenv("FIREBASE_BUCKET_NAME"),
		logger,
	)
	if err != nil {
		log.Fatalf("Failed to initialize firebase service: %v", err)
	}

	analysisService = services.NewAnalysisService(firebaseService, logger)
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

func main() {
	r := gin.Default()

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Welcome to Bounce Rate API!"})
	})

	r.POST("/upload", func(c *gin.Context) {
		filePath := c.PostForm("file_path")
		objectName := c.PostForm("object_name")

		if err := firebaseService.UploadFile(c.Request.Context(), filePath, objectName); err != nil {
			handleError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "File uploaded successfully"})
	})

	r.GET("/download", func(c *gin.Context) {
		objectName := c.Query("object_name")
		destPath := c.Query("dest_path")

		if objectName == "" || destPath == "" {
			c.JSON(400, gin.H{"error": "object_name and dest_path query parameters are required"})
			return
		}

		if err := firebaseService.DownloadFile(c.Request.Context(), objectName, destPath); err != nil {
			handleError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "File downloaded successfully"})
	})

	r.POST("/analyze-purchase", func(c *gin.Context) {
		var request struct {
			ProductType   string  `json:"productType" binding:"required"`
			PurchasePrice float64 `json:"purchasePrice" binding:"required"`
			Location      string  `json:"location" binding:"required"`
		}

		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		averagePrice, err := analysisService.CalculateAveragePrice(c.Request.Context(), request.Location, request.ProductType)
		if err != nil {
			handleError(c, err)
			return
		}

		breakEvenPoint, err := analysisService.CalculateBreakEvenPoint(request.PurchasePrice, averagePrice)
		if err != nil {
			handleError(c, err)
			return
		}

		c.JSON(200, gin.H{
			"message":        "Successfully analyzed purchase",
			"averagePrice":   averagePrice,
			"breakEvenPoint": breakEvenPoint,
		})
	})

	r.GET("/search", func(c *gin.Context) {
		location := c.Query("location")
		if location == "" {
			c.JSON(400, gin.H{"error": "location query parameter is required"})
			return
		}

		result, err := competitorService.SearchCompetitors(c.Request.Context(), location)
		if err != nil {
			handleError(c, err)
			return
		}

		c.JSON(200, result)
	})

	// Get the PORT from environment variables
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default port if not specified
	}

	// Start the Gin server on the specified port
	r.Run(":" + port)
}
