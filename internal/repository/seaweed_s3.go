package repository

import (
	"bytes"
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	appConfig "github.com/mansoorceksport/metamorph/internal/config"
)

// SeaweedS3Repository implements domain.FileRepository using AWS SDK v2
type SeaweedS3Repository struct {
	client    *s3.Client
	bucket    string
	publicURL string
}

// NewSeaweedS3Repository creates a new S3 repository
func NewSeaweedS3Repository(ctx context.Context, cfg appConfig.S3Config) (*SeaweedS3Repository, error) {
	// Load AWS configuration
	// For SeaweedFS (or generic S3), we need to override the endpoint resolution
	// We use static credentials "any"/"any" because SeaweedFS/MinIO often require signatures
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("any", "any", "")),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config, %v", err)
	}

	// Create S3 client
	// We use the functional options pattern for the client to override the endpoint
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = true // Required for many S3-compatible stores including SeaweedFS
	})

	repo := &SeaweedS3Repository{
		client:    client,
		bucket:    cfg.Bucket,
		publicURL: cfg.Endpoint, // Assuming public access is via the same endpoint for now
	}

	// Ensure bucket exists
	if err := repo.ensureBucket(ctx); err != nil {
		return nil, err
	}

	return repo, nil
}

// Upload saves a file to S3 and returns the URL
func (r *SeaweedS3Repository) Upload(ctx context.Context, file []byte, filename string, contentType string) (string, error) {
	key := filename

	_, err := r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(r.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(file),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload file to S3: %w", err)
	}

	// Construct URL
	// Format: {Endpoint}/{Bucket}/{Key}
	url := fmt.Sprintf("%s/%s/%s", r.publicURL, r.bucket, key)
	return url, nil
}

// ensureBucket checks if bucket exists, creating it if necessary
func (r *SeaweedS3Repository) ensureBucket(ctx context.Context) error {
	_, err := r.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(r.bucket),
	})

	if err != nil {
		// If bucket doesn't exist (checking specific error types handles this more robustly,
		// but generic check covers most cases for 404/access denied pattern)
		// Try to create it
		_, err = r.client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(r.bucket),
		})
		if err != nil {
			return fmt.Errorf("failed to create bucket %s: %w", r.bucket, err)
		}
	}
	return nil
}
