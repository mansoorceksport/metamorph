# HOM Gym - Digitizer Service

A Go backend service for digitizing InBody 270 body composition scans using AI (Gemini 1.5 Pro via OpenRouter).

## Features

- 🔐 **Firebase Authentication**: Secure JWT-based user authentication
- 🤖 **AI-Powered Extraction**: Automatic metric extraction from InBody scans using Gemini 2.0 Flash
- 📊 **Segmental Analysis (V2)**: Detailed body composition for 5 body segments (arms, legs, trunk)
- 💬 **Personalized Feedback (V2)**: AI-generated analysis from House of Metamorfit personal trainer
- 📈 **Trend Analysis (V2)**: Compares with previous scan to track progress
- 💾 **MongoDB Storage**: Persistent storage of scan records
- 📦 **SeaweedFS S3 Storage**: Distributed object storage for scan images
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
│   ├── repository/              # MongoDB, Redis & S3 implementations
│   ├── service/                 # Business logic & AI integration
│   ├── middleware/              # Firebase Auth middleware
│   ├── handler/                 # HTTP handlers
│   └── config/                  # Configuration management
└── docs/
    └── implementation_plan.md   # Detailed architecture documentation
```

## Tech Stack

- **Runtime**: Go 1.21+
- **HTTP Framework**: Fiber v2
- **Storage**: MongoDB (Metadata), SeaweedFS (Object Storage/S3)
- **Caching**: Redis
- **Authentication**: Firebase Admin SDK
- **AI**: OpenRouter (Gemini 2.0 Flash)
- **Containerization**: Docker & Docker Compose

## Prerequisites

- Go 1.21+
- Docker & Docker Compose
- Firebase Project (Service Account)
- OpenRouter API Key

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
   ```

   Key environment variables:
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
   OPENROUTER_MODEL=google/gemini-2.0-flash-001

   # S3 (SeaweedFS)
   S3_ENDPOINT=http://127.0.0.1:8333
   S3_REGION=us-east-1
   S3_BUCKET=inbody-scans
   ```

   **Note**: To base64 encode your Firebase private key:
   ```bash
   echo -n "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----" | base64
   ```

3. **Start Infrastructure**
   ```bash
   docker-compose up -d
   ```

   This starts:
   - **MongoDB** on port `27017`
   - **Redis** on port `6379`
   - **SeaweedFS**:
     - S3 API on port `8333`
     - Volume Server on port `8334`
     - Filer UI on port `8899` ([http://localhost:8899](http://localhost:8899))

4. **Run the Application**
   ```bash
   go run cmd/main.go
   ```

   Expected startup logs:
   ```
   ✓ Firebase initialized
   ✓ MongoDB connected
   ✓ Redis connected
   ✓ SeaweedFS S3 repository initialized
   ✓ Services initialized
   ✓ Handlers initialized
   🚀 Server starting on port 8080
   ```

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

### Create Scan (Digitize)
```
POST /v1/scans/digitize
Authorization: Bearer <firebase_jwt_token>
Content-Type: multipart/form-data
```

**Request**:
- `image`: Image file (JPEG/PNG/HEIC, max 5MB)

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
    "visceral_fat": 8,
    "whr": 0.85,
    "segmental_lean": {
      "right_arm": {"mass": 2.8, "percentage": 8.0},
      "left_arm": {"mass": 2.7, "percentage": 7.8},
      "trunk": {"mass": 22.5, "percentage": 63.9},
      "right_leg": {"mass": 8.1, "percentage": 23.0},
      "left_leg": {"mass": 8.2, "percentage": 23.3}
    },
    "segmental_fat": {
      "right_arm": {"mass": 0.6, "percentage": 4.7},
      "left_arm": {"mass": 0.6, "percentage": 4.7},
      "trunk": {"mass": 7.2, "percentage": 56.3},
      "right_leg": {"mass": 2.2, "percentage": 17.2},
      "left_leg": {"mass": 2.2, "percentage": 17.2}
    },
    "analysis": {
      "summary": "Your body composition shows good muscle mass with healthy visceral fat levels.",
      "positive_feedback": [
        "Excellent visceral fat level of 8",
        "Strong skeletal muscle mass at 35.2kg"
      ],
      "improvements": [
        "Body fat percentage could be reduced for enhanced definition"
      ],
      "advice": [
        "Maintain current strength training routine",
        "Add 2-3 HIIT sessions per week"
      ]
    },
    "metadata": {
      "image_url": "http://127.0.0.1:8333/inbody-scans/user123/1234567890.jpg",
      "processed_at": "2025-12-24T17:30:00Z"
    }
  }
}
```

**Note**: V2 fields (`segmental_lean`, `segmental_fat`, `analysis`) are optional and only included when visible in the scan.

### List All Scans
```
GET /v1/scans
Authorization: Bearer <firebase_jwt_token>
```

**Response**:
```json
{
  "success": true,
  "data": [
    {
      "id": "676a1b2c3d4e5f6g7h8i9j0k",
      "user_id": "firebase_user_uid",
      "test_date_time": "2025-12-24T10:00:00Z",
      "weight": 75.5,
      ...
    }
  ]
}
```

### Get Single Scan
```
GET /v1/scans/:id
Authorization: Bearer <firebase_jwt_token>
```

**Response**:
```json
{
  "success": true,
  "data": {
    "id": "676a1b2c3d4e5f6g7h8i9j0k",
    ...
  }
}
```

**Errors**:
- `404`: Scan not found
- `403`: Access denied (not your scan)

### Update Scan Metrics
```
PATCH /v1/scans/:id
Authorization: Bearer <firebase_jwt_token>
Content-Type: application/json
```

**Request Body** (all fields optional):
```json
{
  "weight": 76.0,
  "smm": 35.5,
  "body_fat_mass": 12.5,
  "pbf": 16.5,
  "bmi": 22.6,
  "bmr": 1660,
  "visceral_fat": 7,
  "whr": 0.84
}
```

**Response**:
```json
{
  "success": true,
  "data": {
    "id": "676a1b2c3d4e5f6g7h8i9j0k",
    ...
  }
}
```

**Errors**:
- `404`: Scan not found
- `403`: Access denied (not your scan)

### Delete Scan
```
DELETE /v1/scans/:id
Authorization: Bearer <firebase_jwt_token>
```

**Response**:
```json
{
  "success": true,
  "message": "scan deleted successfully"
}
```

**Note**: This also deletes the associated image from SeaweedFS storage.

**Errors**:
- `404`: Scan not found
- `403`: Access denied (not your scan)

### Analytics History (for Charting)
```
GET /v1/analytics/history?limit=10
Authorization: Bearer <firebase_jwt_token>
```

**Query Parameters**:
- `limit` (optional): Number of scans to retrieve (default: 10, max: 100)

**Response**:
```json
{
  "success": true,
  "data": {
    "progress": {
      "total_scans": 8,
      "first_scan_date": "2025-01-15",
      "latest_scan_date": "2025-12-20",
      "weight_change": -2.5,
      "muscle_gained": 1.5,
      "body_fat_change": -3.2
    },
    "history": [
      {
        "date": "2025-01-15T10:00:00Z",
        "core_metrics": {
          "weight": 78.0,
          "smm": 33.7,
          "pbf": 20.1
        },
        "segmental_trends": {
          "lean": {
            "right_arm": {"mass": 2.7, "percentage": 8.0},
            "left_arm": {"mass": 2.6, "percentage": 7.7}
          }
        }
      }
    ]
  }
}
```

**Use Cases**:
- 📊 **"Big Three" Chart**: Plot Weight, SMM, PBF over time
- 📊 **Segmental Balance**: Track right vs left arm muscle balance
- 🏆 **Progress Summary**: Show total gains since joining HOM Gym

## API Documentation

The complete API specification is available as an OpenAPI 3.0 document:

📄 **[OpenAPI Spec](docs/openapi.yaml)**

### Using with Postman

1. Open Postman
2. Click **Import** → **File**
3. Select `docs/openapi.yaml`
4. Postman will create a collection with all endpoints pre-configured

### Using with Other Tools

The OpenAPI spec is compatible with:
- **Swagger UI**: Interactive API documentation
- **Insomnia**: REST client
- **Any OpenAPI 3.0 compatible tool**

## Cost Optimization

The service uses **Gemini 2.0 Flash** (via OpenRouter) for optimal cost and performance:
- Extremely fast inference
- Low cost per request
- Excellent accuracy for structured data extraction

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

## Infrastructure

### Docker Services

| Service | Port | Purpose |
|---------|------|----------|
| MongoDB | 27017 | NoSQL database for metadata |
| Redis | 6379 | Cache layer for recent scans |
| SeaweedFS S3 | 8333 | S3-compatible object storage API |
| SeaweedFS Volume | 8334 | Internal volume server |
| SeaweedFS Filer UI | 8899 | Web UI to browse stored files |

### SeaweedFS File Browser

Access the SeaweedFS UI at [http://localhost:8899](http://localhost:8899) to browse uploaded scan images.
Navigate to `/buckets/inbody-scans/` to see your files.

## Troubleshooting

### SeaweedFS Connection Issues

If you see `connection reset` errors:

1. **Use IPv4 explicitly** in `.env`:
   ```bash
   S3_ENDPOINT=http://127.0.0.1:8333  # Not localhost
   ```

2. **Verify containers are running**:
   ```bash
   docker ps
   ```

3. **Check SeaweedFS logs**:
   ```bash
   docker logs metamorph-seaweedfs-1
   ```

4. **Restart infrastructure**:
   ```bash
   docker-compose down && docker-compose up -d
   ```

### Port Conflicts

If ports are already in use, update `docker-compose.yaml` to remap:
- Filer UI is on `8899` (was `8888` to avoid Jupyter conflict)

## License

Copyright © 2025 mansoorceksport
