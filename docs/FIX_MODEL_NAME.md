# Quick Fix: Update OpenRouter Model Name

## Issue
OpenRouter returned error: `google/gemini-1.5-pro is not a valid model ID`

## Solution

### Step 1: Update Your `.env` File

Open your `.env` file and find this line:
```bash
OPENROUTER_MODEL=google/gemini-1.5-pro
```

Change it to:
```bash
OPENROUTER_MODEL=google/gemini-flash-1.5
```

### Step 2: Restart Your Server

1. Stop the current server (Ctrl+C in the terminal running the server)
2. Start it again:
   ```bash
   go run cmd/main.go
   ```

### Step 3: Test Again

Run your curl command again:
```bash
curl -X POST http://localhost:8080/v1/scans/digitize \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "image=@/path/to/inbody-scan.jpg"
```

## Bonus: Cost Savings! 💰

By using `google/gemini-flash-1.5` instead of `google/gemini-pro-1.5`:
- **17x cheaper** for the same vision capabilities!
- Input: $0.075/M tokens (vs $1.25/M)
- Output: $0.30/M tokens (vs $2.50/M)

If you need more advanced reasoning later, you can switch to:
```bash
OPENROUTER_MODEL=google/gemini-pro-1.5
```

## Available Models

- `google/gemini-flash-1.5` - Fast, cheap, perfect for InBody scans ✅
- `google/gemini-pro-1.5` - More expensive, better for complex reasoning
