package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mansoorceksport/metamorph/internal/domain"
)

const (
	openRouterAPIURL = "https://openrouter.ai/api/v1/chat/completions"
	systemPrompt     = "You are an expert at extracting data from InBody 270 body composition scans. Extract the requested metrics accurately and return only valid JSON."
	userPrompt       = `Extract weight, smm, body_fat_mass, pbf, bmi, bmr, visceral_fat, whr (waist-hip ratio), and test_date from this InBody 270 scan. 
Return ONLY JSON in this exact format:
{
  "weight": 0.0,
  "smm": 0.0,
  "body_fat_mass": 0.0,
  "pbf": 0.0,
  "bmi": 0.0,
  "bmr": 0,
  "visceral_fat": 0,
  "whr": 0.0,
  "test_date": "2025-12-24T10:00:00Z"
}`
)

// OpenRouterDigitizer implements domain.DigitizerService using OpenRouter API
type OpenRouterDigitizer struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewOpenRouterDigitizer creates a new OpenRouter digitizer service
func NewOpenRouterDigitizer(apiKey, model string) *OpenRouterDigitizer {
	return &OpenRouterDigitizer{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // AI processing can take some time
		},
	}
}

// ExtractMetrics uses OpenRouter AI to extract InBody metrics from an image
func (d *OpenRouterDigitizer) ExtractMetrics(ctx context.Context, imageData []byte) (*domain.InBodyMetrics, error) {
	// Detect image type from file header
	imageType := detectImageType(imageData)

	// Encode image to base64
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	// Build request payload
	requestBody := map[string]interface{}{
		"model": d.model,
		"messages": []map[string]interface{}{
			{
				"role":    "system",
				"content": systemPrompt,
			},
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": userPrompt,
					},
					{
						"type": "image_url",
						"image_url": map[string]string{
							"url": fmt.Sprintf("data:%s;base64,%s", imageType, imageBase64),
						},
					},
				},
			},
		},
		"temperature": 0.1, // Low temperature for consistent, factual extraction
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", openRouterAPIURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+d.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://homgym.app") // Optional: for OpenRouter analytics
	req.Header.Set("X-Title", "HOM Gym Digitizer")       // Optional: for OpenRouter analytics

	// Send request
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for non-200 status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openrouter api error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse OpenRouter response
	var apiResponse struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message  string                 `json:"message"`
			Code     int                    `json:"code"`
			Metadata map[string]interface{} `json:"metadata"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for API error with detailed message
	if apiResponse.Error != nil {
		errorMsg := fmt.Sprintf("openrouter error: %s (code: %d)", apiResponse.Error.Message, apiResponse.Error.Code)
		if apiResponse.Error.Metadata != nil {
			if providerErr, ok := apiResponse.Error.Metadata["provider_error"].(string); ok {
				errorMsg += fmt.Sprintf(" - Provider error: %s", providerErr)
			}
			if raw, ok := apiResponse.Error.Metadata["raw"].(string); ok {
				errorMsg += fmt.Sprintf(" - Raw: %s", raw)
			}
		}
		return nil, fmt.Errorf(errorMsg)
	}

	// Check if we got a response
	if len(apiResponse.Choices) == 0 {
		return nil, fmt.Errorf("no response from AI model")
	}

	// Extract the JSON content from the AI response
	content := apiResponse.Choices[0].Message.Content

	// Parse the metrics JSON
	var metrics domain.InBodyMetrics
	if err := json.Unmarshal([]byte(content), &metrics); err != nil {
		// If direct unmarshal fails, try to find JSON in the response
		// Sometimes AI adds explanation text around the JSON
		metrics, err = extractJSONFromText(content)
		if err != nil {
			return nil, fmt.Errorf("failed to parse AI response as JSON: %w (response: %s)", err, content)
		}
	}

	return &metrics, nil
}

// extractJSONFromText attempts to find and parse JSON from text that may contain other content
func extractJSONFromText(text string) (domain.InBodyMetrics, error) {
	var metrics domain.InBodyMetrics

	// Find JSON object in text (simple approach: look for first { and last })
	start := bytes.IndexByte([]byte(text), '{')
	end := bytes.LastIndexByte([]byte(text), '}')

	if start == -1 || end == -1 || start >= end {
		return metrics, fmt.Errorf("no JSON object found in text")
	}

	jsonStr := text[start : end+1]
	if err := json.Unmarshal([]byte(jsonStr), &metrics); err != nil {
		return metrics, err
	}

	return metrics, nil
}

// detectImageType detects the MIME type of an image from its header bytes
func detectImageType(data []byte) string {
	if len(data) < 12 {
		return "image/jpeg" // default fallback
	}

	// Check for JPEG (FF D8 FF)
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}

	// Check for PNG (89 50 4E 47)
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "image/png"
	}

	// Check for GIF (47 49 46)
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 {
		return "image/gif"
	}

	// Check for WebP (52 49 46 46 ... 57 45 42 50)
	if len(data) >= 12 && data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
		return "image/webp"
	}

	// Default to JPEG if unknown
	return "image/jpeg"
}
