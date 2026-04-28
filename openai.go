package captcha

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const defaultOpenAIBaseURL = "https://api.openai.com"

type openaiProvider struct {
	apiKey  string
	baseURL string
	model   string
}

func newOpenAIProvider(cfg Config) (*openaiProvider, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("openai: Model is required")
	}

	base := cfg.BaseURL
	if base == "" {
		base = defaultOpenAIBaseURL
	}

	return &openaiProvider{
		apiKey:  cfg.APIKey,
		baseURL: base,
		model:   cfg.Model,
	}, nil
}

func (o *openaiProvider) Name() string { return "openai" }

func (o *openaiProvider) Call(imageData []byte, prompt string) (string, error) {
	b64 := base64.StdEncoding.EncodeToString(imageData)

	reqBody := openaiRequest{
		Model: o.model,
		Messages: []openaiMessage{{
			Role: "user",
			Content: []openaiContentPart{
				{Type: "text", Text: prompt},
				{Type: "image_url", ImageURL: &openaiImageURL{
					URL: "data:image/jpeg;base64," + b64,
				}},
			},
		}},
		MaxTokens:   defaultMaxTokens,
		Temperature: floatPtr(0),
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	apiURL := buildURL(o.baseURL, "/v1/chat/completions")

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request: %w", err)
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	statusCode := resp.StatusCode
	retryAfter := resp.Header.Get("Retry-After")
	resp.Body.Close()

	if statusCode == 429 {
		wait := parseRetryAfter(retryAfter, rpmWindow)
		log.Printf("captcha solver [openai]: rate limited, waiting %v", wait.Round(time.Second))
		return "", &RateLimitError{Wait: wait, Message: "openai: rate limited"}
	}

	if statusCode == 401 || statusCode == 403 {
		return "", &AuthError{Message: fmt.Sprintf("openai: auth failed (HTTP %d)", statusCode)}
	}

	if statusCode != 200 {
		var oe openaiErrorResponse
		json.Unmarshal(body, &oe)
		return "", fmt.Errorf("openai: HTTP %d - %s", statusCode, truncate(oe.Error.Message, 100))
	}

	var gr openaiResponse
	if err := json.Unmarshal(body, &gr); err != nil {
		return "", fmt.Errorf("openai: parse response: %w", err)
	}

	if len(gr.Choices) == 0 {
		return "", fmt.Errorf("openai: empty response")
	}

	choice := gr.Choices[0]
	if choice.FinishReason != "" && choice.FinishReason != "stop" {
		return "", fmt.Errorf("openai: incomplete response: %s", choice.FinishReason)
	}

	return choice.Message.Content, nil
}

type openaiRequest struct {
	Model       string          `json:"model"`
	Messages    []openaiMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature *float64        `json:"temperature,omitempty"`
}

type openaiMessage struct {
	Role    string              `json:"role"`
	Content []openaiContentPart `json:"content"`
}

type openaiContentPart struct {
	Type     string         `json:"type"`
	Text     string         `json:"text,omitempty"`
	ImageURL *openaiImageURL `json:"image_url,omitempty"`
}

type openaiImageURL struct {
	URL string `json:"url"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
}

type openaiChoice struct {
	Message      openaiMessageResponse `json:"message"`
	FinishReason string                `json:"finish_reason"`
}

type openaiMessageResponse struct {
	Content string `json:"content"`
}

type openaiErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func floatPtr(f float64) *float64 { return &f }
