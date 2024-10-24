package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"cloud.google.com/go/storage"
	firebase "firebase.google.com/go"
	"google.golang.org/api/option"
)

type FirebaseService struct {
	app     *firebase.App
	storage *storage.Client
	bucket  *storage.BucketHandle
	logger  *log.Logger
}

type Location struct {
	Name        string       `json:"name"`
	Competitors []Competitor `json:"competitors"`
}

func NewFirebaseService(credentialsFilePath, bucketName string, logger *log.Logger) (*FirebaseService, error) {
	// Initialize Firebase app
	opt := option.WithCredentialsFile(credentialsFilePath)
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		return nil, fmt.Errorf("error initializing firebase app: %v", err)
	}

	// Initialize Firebase Storage client
	storageClient, err := storage.NewClient(context.Background(), opt)
	if err != nil {
		return nil, fmt.Errorf("error initializing firebase storage client: %v", err)
	}

	bucket := storageClient.Bucket(bucketName)

	return &FirebaseService{
		app:     app,
		storage: storageClient,
		bucket:  bucket,
		logger:  logger,
	}, nil
}

func (fs *FirebaseService) UploadFile(ctx context.Context, filePath, objectName string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("error opening file: %v", err)
	}
	defer f.Close()

	wc := fs.bucket.Object(objectName).NewWriter(ctx)
	if _, err = io.Copy(wc, f); err != nil {
		return fmt.Errorf("error uploading file to firebase storage: %v", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("error closing writer: %v", err)
	}

	fs.logger.Printf("File %s uploaded to %s", filePath, objectName)
	return nil
}

func (fs *FirebaseService) DownloadFile(ctx context.Context, objectName, destPath string) error {
	rc, err := fs.bucket.Object(objectName).NewReader(ctx)
	if err != nil {
		return fmt.Errorf("error creating reader: %v", err)
	}
	defer rc.Close()

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("error creating file: %v", err)
	}
	defer f.Close()

	if _, err = io.Copy(f, rc); err != nil {
		return fmt.Errorf("error downloading file from firebase storage: %v", err)
	}

	fs.logger.Printf("File %s downloaded to %s", objectName, destPath)
	return nil
}

func (fs *FirebaseService) StoreLocation(ctx context.Context, location Location) error {
	locationData, err := json.Marshal(location)
	if err != nil {
		return fmt.Errorf("error marshaling location data: %v", err)
	}

	objectName := fmt.Sprintf("%s/location.json", location.Name)
	wc := fs.bucket.Object(objectName).NewWriter(ctx)
	if _, err = wc.Write(locationData); err != nil {
		return fmt.Errorf("error writing location data to firebase storage: %v", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("error closing writer: %v", err)
	}

	fs.logger.Printf("Location %s stored in %s", location.Name, objectName)
	return nil
}

func (fs *FirebaseService) StoreCompetitor(ctx context.Context, locationName string, competitor Competitor) error {
	competitorData, err := json.Marshal(competitor)
	if err != nil {
		return fmt.Errorf("error marshaling competitor data: %v", err)
	}

	objectName := fmt.Sprintf("%s/%s/competitor", locationName, competitor.Name)
	wc := fs.bucket.Object(objectName).NewWriter(ctx)
	if _, err = wc.Write(competitorData); err != nil {
		return fmt.Errorf("error writing competitor data to firebase storage: %v", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("error closing writer: %v", err)
	}

	fs.logger.Printf("Competitor %s stored in %s", competitor.Name, objectName)
	return nil
}

func (fs *FirebaseService) StoreProduct(ctx context.Context, locationName, competitorName, category string, product Product) error {
	productData, err := json.Marshal(product)
	if err != nil {
		return fmt.Errorf("error marshaling product data: %v", err)
	}

	objectName := fmt.Sprintf("%s/%s/%s/%s.json", locationName, competitorName, category, product.Name)
	wc := fs.bucket.Object(objectName).NewWriter(ctx)
	if _, err = wc.Write(productData); err != nil {
		return fmt.Errorf("error writing product data to firebase storage: %v", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("error closing writer: %v", err)
	}

	fs.logger.Printf("Product %s stored in %s", product.Name, objectName)
	return nil
}

func (fs *FirebaseService) GetLocation(ctx context.Context, locationName string) (*Location, error) {
	objectName := fmt.Sprintf("%s/location.json", locationName)
	rc, err := fs.bucket.Object(objectName).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("error creating reader: %v", err)
	}
	defer rc.Close()

	var location Location
	if err := json.NewDecoder(rc).Decode(&location); err != nil {
		return nil, fmt.Errorf("error decoding location data: %v", err)
	}

	fs.logger.Printf("Location %s retrieved from %s", locationName, objectName)
	return &location, nil
}
