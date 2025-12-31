package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"text/template"
	"time"

	"github.com/mansoorceksport/metamorph/internal/domain"
)

const (
	openRouterAPIURL = "https://openrouter.ai/api/v1/chat/completions"

	// Default values if no tenant context is found
	defaultGymName = "House of Metamorfit (HOM)"
	defaultTone    = "Encouraging, empathetic"
	defaultStyle   = "Tactical, practical"
	defaultPersona = "Supportive personal trainer"

	// System Prompt Template
	systemPromptTmplStr = `You are an expert at extracting data from InBody 270 body composition scans AND a {{.Persona}} from '{{.GymName}}'. Your tone should be {{.Tone}}. Your advice should be {{.Style}}. Extract metrics accurately and provide coaching advice. Return only valid JSON.`

	// User Prompt: Analysis Section Template
	analysisPromptTmplStr = `
**ANALYSIS TASK - {{.Persona}} from {{.GymName}}:**

You are a {{.Persona}} at {{.GymName}}. Provide:

1. **Summary** (2-3 sentences):
   - Start with a {{.Tone}} tone
   - Acknowledge their progress
   - Mention 1-2 key observations
   - Reference {{.GymName}} as their community

2. **Positive Feedback** (2-3 items):
   - Highlight strengths
   - Be specific with numbers
   - Make them feel proud

3. **Improvements** (2-3 items):
   - Identify areas for growth, phrased constructively
   - **ASYMMETRY DETECTION**: Compare segmental lean percentages (>2%% diff)
   - **METRIC CORRELATION**: Visceral Fat >10 (Suggest HIIT/Zone 2), PBF High (Suggest caloric deficit)

4. **Advice** (2-3 actionable items):
   - Provide {{.Style}} advice they can implement at {{.GymName}}
   - **For Asymmetries**: Recommend unilateral exercises
   - **For Visceral Fat**: Suggest cardio (Zone 2/HIIT)
   - **For Muscle**: Progressive overload
`
)

// PromptContext holds data for the templates
type PromptContext struct {
	GymName string
	Tone    string
	Style   string
	Persona string
}

// OpenRouterDigitizer implements domain.DigitizerService using OpenRouter API
type OpenRouterDigitizer struct {
	apiKey       string
	model        string
	httpClient   *http.Client
	userRepo     domain.UserRepository
	tenantRepo   domain.TenantRepository
	systemTmpl   *template.Template
	analysisTmpl *template.Template
}

// NewOpenRouterDigitizer creates a new OpenRouter digitizer service
func NewOpenRouterDigitizer(
	apiKey, model string,
	userRepo domain.UserRepository,
	tenantRepo domain.TenantRepository,
) *OpenRouterDigitizer {
	// Parse templates on init
	sysTmpl, _ := template.New("system").Parse(systemPromptTmplStr)
	anaTmpl, _ := template.New("analysis").Parse(analysisPromptTmplStr)

	return &OpenRouterDigitizer{
		apiKey:       apiKey,
		model:        model,
		httpClient:   &http.Client{Timeout: 60 * time.Second},
		userRepo:     userRepo,
		tenantRepo:   tenantRepo,
		systemTmpl:   sysTmpl,
		analysisTmpl: anaTmpl,
	}
}

// ExtractMetrics uses OpenRouter AI to extract InBody metrics from an image
func (d *OpenRouterDigitizer) ExtractMetrics(ctx context.Context, userID string, imageData []byte) (*domain.InBodyMetrics, error) {
	// 1. Determine Context (SaaS)
	promptCtx := PromptContext{
		GymName: defaultGymName,
		Tone:    defaultTone,
		Style:   defaultStyle,
		Persona: defaultPersona,
	}

	// Try to fetch user and tenant
	if userID != "" && d.userRepo != nil {
		user, err := d.userRepo.GetByFirebaseUID(ctx, userID)
		if err == nil && user != nil && user.TenantID != "" {
			tenant, err := d.tenantRepo.GetByID(ctx, user.TenantID)
			if err == nil && tenant != nil {
				promptCtx.GymName = tenant.Name
				if tenant.AISettings.Tone != "" {
					promptCtx.Tone = tenant.AISettings.Tone
				}
				if tenant.AISettings.Style != "" {
					promptCtx.Style = tenant.AISettings.Style
				}
				if tenant.AISettings.Persona != "" {
					promptCtx.Persona = tenant.AISettings.Persona
				}
			}
		}
	}

	// 2. Generate Prompts
	var systemPromptBuf bytes.Buffer
	if err := d.systemTmpl.Execute(&systemPromptBuf, promptCtx); err != nil {
		return nil, fmt.Errorf("failed to generate system prompt: %w", err)
	}

	var analysisPromptBuf bytes.Buffer
	if err := d.analysisTmpl.Execute(&analysisPromptBuf, promptCtx); err != nil {
		return nil, fmt.Errorf("failed to generate analysis prompt: %w", err)
	}

	// Combine Analysis prompt with the static Extraction prompt
	// Note: We need to include the static extraction part.
	fullUserPrompt := fmt.Sprintf(`Analyze this InBody 270 scan and extract ALL available data, then provide personalized, tactical coaching feedback.

**EXTRACTION TASK:**
1. Extract ALL standard metrics: weight, smm, body_fat_mass, pbf, bmi, bmr, visceral_fat, whr, test_date
2. Extract ADDITIONAL metrics (Look at the **RIGHT COLUMN** of the sheet):
   - **InBody Score**: Found in the top right box (e.g., "74/100"). Extract the numeric score.
   - **Weight Control Section**:
     - Target Weight (kg)
     - Weight Control (kg) -> can be negative (-) or positive (+)
     - Fat Control (kg) -> can be negative (-) or positive (+)
     - Muscle Control (kg) -> can be negative (-) or positive (+)
   - **Research Parameters Section**:
     - Fat Free Mass (kg)
     - Obesity Degree (%%) -> Extract the number (e.g., 105)
     - Recommended Calorie Intake (kcal) -> often labeled "Recommended calorie intake" or under "Research Parameters"
3. Extract SEGMENTAL DATA from the body composition silhouettes (if visible):
   - Segmental Lean Mass (kg and %%) for: right_arm, left_arm, trunk, right_leg, left_leg
   - Segmental Fat Mass (kg and %%) for: right_arm, left_arm, trunk, right_leg, left_leg

%s

**IMPORTANT VALIDATION:**
- If segmental data shows all 0.0 or missing values, acknowledge in the summary:
  "The segmental silhouettes weren't clear enough in this photo to extract detailed body part analysis. For best results, ensure the scan is well-lit and all body composition charts are visible."

Return ONLY valid JSON in this EXACT format:
{
  "weight": 0.0,
  "smm": 0.0,
  "body_fat_mass": 0.0,
  "pbf": 0.0,
  "bmi": 0.0,
  "bmr": 0,
  "visceral_fat": 0,
  "whr": 0.0,
  "test_date": "2025-12-24T10:00:00Z",
  "inbody_score": 0.0,
  "obesity_degree": 0.0,
  "fat_free_mass": 0.0,
  "recommended_calorie_intake": 0,
  "target_weight": 0.0,
  "weight_control": 0.0,
  "fat_control": 0.0,
  "muscle_control": 0.0,
  "segmental_lean": {
    "right_arm": {"mass": 0.0, "percentage": 0.0},
    "left_arm": {"mass": 0.0, "percentage": 0.0},
    "trunk": {"mass": 0.0, "percentage": 0.0},
    "right_leg": {"mass": 0.0, "percentage": 0.0},
    "left_leg": {"mass": 0.0, "percentage": 0.0}
  },
  "segmental_fat": {
    "right_arm": {"mass": 0.0, "percentage": 0.0},
    "left_arm": {"mass": 0.0, "percentage": 0.0},
    "trunk": {"mass": 0.0, "percentage": 0.0},
    "right_leg": {"mass": 0.0, "percentage": 0.0},
    "left_leg": {"mass": 0.0, "percentage": 0.0}
  },
  "analysis": {
    "summary": "Encouraging 2-3 sentence summary with %s community context",
    "positive_feedback": ["specific strength 1 with numbers", "specific strength 2"],
    "improvements": ["area 1 with asymmetry details if >2%%%%", "area 2 with visceral fat context"],
    "advice": ["tactical gym advice 1 (unilateral exercise if needed)", "tactical gym advice 2 (cardio zones if needed)"]
  }
}

NOTE: If segmental data is not visible or unclear, use 0.0 and mention it in the analysis summary.`, analysisPromptBuf.String(), promptCtx.GymName)

	// Detect image type
	imageType := detectImageType(imageData)
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	// Build request payload
	requestBody := map[string]interface{}{
		"model": d.model,
		"messages": []map[string]interface{}{
			{
				"role":    "system",
				"content": systemPromptBuf.String(),
			},
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": fullUserPrompt,
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
		"temperature": 0.1,
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
	req.Header.Set("HTTP-Referer", "https://homgym.app") // Optional
	req.Header.Set("X-Title", "HOM Gym Digitizer")       // Optional

	// Send request
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openrouter api error (status %d): %s", resp.StatusCode, string(body))
	}

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

	if apiResponse.Error != nil {
		errorMsg := fmt.Sprintf("openrouter error: %s (code: %d)", apiResponse.Error.Message, apiResponse.Error.Code)
		if apiResponse.Error.Metadata != nil {
			if providerErr, ok := apiResponse.Error.Metadata["provider_error"].(string); ok {
				errorMsg += fmt.Sprintf(" - Provider error: %s", providerErr)
			}
		}
		return nil, fmt.Errorf(errorMsg)
	}

	if len(apiResponse.Choices) == 0 {
		return nil, fmt.Errorf("no response from AI model")
	}

	content := apiResponse.Choices[0].Message.Content

	var metrics domain.InBodyMetrics
	if err := json.Unmarshal([]byte(content), &metrics); err != nil {
		metrics, err = extractJSONFromText(content)
		if err != nil {
			return nil, fmt.Errorf("failed to parse AI response as JSON: %w", err)
		}
	}

	return &metrics, nil
}

// extractJSONFromText attempts to find and parse JSON from text that may contain other content
func extractJSONFromText(text string) (domain.InBodyMetrics, error) {
	var metrics domain.InBodyMetrics

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
		return "image/jpeg"
	}
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "image/png"
	}
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 {
		return "image/gif"
	}
	if len(data) >= 12 && data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
		return "image/webp"
	}
	return "image/jpeg"
}
