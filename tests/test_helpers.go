package tests

import (
	"context"
	"fmt"
	"log"
	"testing"

	"firebase.google.com/go/v4/auth"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SetupTestDB spins up a fresh MongoDB container and returns the database connection
// along with a cleanup function.
func SetupTestDB(t *testing.T) (*mongo.Database, func()) {
	ctx := context.Background()

	mongodbContainer, err := mongodb.Run(ctx, "mongo:latest")
	if err != nil {
		t.Fatalf("failed to start container: %s", err)
	}

	endpoint, err := mongodbContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("failed to get connection string: %s", err)
	}

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(endpoint))
	if err != nil {
		t.Fatalf("failed to connect to mongo: %v", err)
	}

	return mongoClient.Database("test_db"), func() {
		if err := mongoClient.Disconnect(ctx); err != nil {
			log.Printf("failed to disconnect mongo: %v", err)
		}
		if err := mongodbContainer.Terminate(ctx); err != nil {
			log.Printf("failed to terminate container: %v", err)
		}
	}
}

// MockAuthClient implements service.FirebaseAuthClient for testing
type MockAuthClient struct {
	// Map of ID Tokens to Mocked User Records (or minimal data needed)
	// Key: ID Token provided in header
	// Value: *auth.Token (what VerifyIDToken returns)
	ValidTokens map[string]*auth.Token
}

func NewMockAuthClient() *MockAuthClient {
	return &MockAuthClient{
		ValidTokens: make(map[string]*auth.Token),
	}
}

func (m *MockAuthClient) VerifyIDToken(ctx context.Context, idToken string) (*auth.Token, error) {
	if token, ok := m.ValidTokens[idToken]; ok {
		return token, nil
	}
	return nil, fmt.Errorf("invalid mock token")
}

// Helper to create a mock token
func (m *MockAuthClient) AddMockUser(tokenString string, uid string, email string) {
	m.ValidTokens[tokenString] = &auth.Token{
		UID: uid,
		Claims: map[string]interface{}{
			"email": email,
			// Add other claims if necessary
		},
	}
}
