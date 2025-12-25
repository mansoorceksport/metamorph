package domain

import (
	"context"
)

// FileRepository defines the interface for file storage operations
type FileRepository interface {
	// Upload saves a file and returns its access URL
	Upload(ctx context.Context, file []byte, filename string, contentType string) (string, error)
}
