# HOM Gym - Digitizer Service

A Go backend service for digitizing InBody 270 body composition scans using AI (Gemini 1.5 Pro via OpenRouter).

## Features

- 🔐 **Firebase Authentication**: Secure JWT-based user authentication
- 🤖 **AI-Powered Extraction**: Automatic metric extraction from InBody scans using Gemini 1.5 Pro
- 💾 **MongoDB Storage**: Persistent storage of scan records
- ⚡ **Redis Caching**: 24-hour caching of latest scans for fast dashboard loading
- 🏗️ **Clean Architecture**: Modular structure with dependency inversion
- 🚀 **Fiber Framework**: High-performance HTTP framework

## Architecture

```
/Users/mansoor/go/src/github.com/mansoorceksport/metamorph/
├── cmd/
│   └── main.go                  # Application entry point
├── internal/
│   ├── domain/                  # Domain models & interfaces
│   ├── repository/              # MongoDB & Redis implementations
│   ├── service/                 # Business logic & AI integration
│   ├── middleware/              # Firebase Auth middleware
│   ├── handler/                 # HTTP handlers
│   └── config/                  # Configuration management
└── docs/
    └── implementation_plan.md   # Detailed architecture documentation
```

## Prerequisites

- **Storage**: MongoDB (Metadata), SeaweedFS (Object Storage/S3)
- **Caching**: Redis
- **Containerization**: Docker & Docker Compose

## Prerequisites

- Go 1.21+
- Docker & Docker Compose
- Firebase Project (Service Account)
- OpenRouter API Key
- SeaweedFS (Included in Docker Compose)

## Setup

1. **Clone the repository**
   ```bash
   git clone https://github.com/mansoorceksport/metamorph.git
   cd metamorph
   ```

2. **Configure Environment**
   ```bash
   cp .env.example .env
   # Edit .env with your credentials
   # Set S3_ENDPOINT=http://127.0.0.1:8333 for SeaweedFS
   ```env
# Server
PORT=8080
MAX_UPLOAD_SIZE_MB=5

# MongoDB
MONGODB_URI=mongodb://localhost:27017
MONGODB_DATABASE=homgym

# Redis
REDIS_ADDR=localhost:6379

# Firebase (from your service account JSON)
FIREBASE_PROJECT_ID=your-project-id
FIREBASE_PRIVATE_KEY=<base64 encoded private key>
FIREBASE_CLIENT_EMAIL=firebase-adminsdk@your-project.iam.gserviceaccount.com

# OpenRouter
OPENROUTER_API_KEY=your_api_key
OPENROUTER_MODEL=google/gemini-1.5-pro
```

**Note**: To base64 encode your Firebase private key:
```bash
echo -n "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----" | base64
```

### 3. Start Dependencies

```bash
docker-compose up -d
```

### 4. Run the Application

```bash
go run cmd/main.go
```

The server will start on `http://localhost:8080`

## API Endpoints

### Health Check
```
GET /health
```

Response:
```json
{
  "status": "healthy",
  "service": "hom-gym-digitizer"
}
```

### Digitize InBody Scan
```
POST /v1/scans/digitize
Authorization: Bearer <firebase_jwt_token>
Content-Type: multipart/form-data
```

**Request**:
- `image`: Image file (JPEG/PNG, max 5MB)

**Response**:
```json
{
  "success": true,
  "data": {
    "id": "676a1b2c3d4e5f6g7h8i9j0k",
    "user_id": "firebase_user_uid",
    "test_date_time": "2025-12-24T10:00:00Z",
    "weight": 75.5,
    "smm": 35.2,
    "body_fat_mass": 12.8,
    "bmi": 22.5,
    "pbf": 16.9,
    "bmr": 1650,
    "metadata": {
      "image_url": "scan.jpg",
      "processed_at": "2025-12-24T17:30:00Z"
    }
  }
}
```

## Cost Optimization

The service uses **Gemini 1.5 Pro** which is the most cost-effective vision model on OpenRouter:
- Input: $1.25 per million tokens (~5x cheaper than Claude)
- Output: $2.50 per million tokens (~12x cheaper than Claude)

## Testing

### Manual Test with cURL

```bash
# Get a Firebase token from your frontend/client first
TOKEN="your_firebase_id_token"

curl -X POST http://localhost:8080/v1/scans/digitize \
  -H "Authorization: Bearer $TOKEN" \
  -F "image=@path/to/inbody-scan.jpg"
```

## Development

### Build
```bash
go build -o bin/metamorph ./cmd/main.go
```

### Run Tests
```bash
go test ./...
```

## Project Structure

This project follows **Clean Architecture** principles:

1. **Domain Layer** (`internal/domain/`): Core business entities and interfaces
2. **Repository Layer** (`internal/repository/`): Data persistence implementations
3. **Service Layer** (`internal/service/`): Business logic and external integrations
4. **Handler Layer** (`internal/handler/`): HTTP request handling
5. **Middleware Layer** (`internal/middleware/`): Cross-cutting concerns (auth)

Dependencies flow inward: Handlers → Services → Repositories → Domain

## License

Copyright © 2025 mansoorceksport
