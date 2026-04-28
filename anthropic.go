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

const defaultAnthropicBaseURL = "https://api.anthropic.com"

type anthropicProvider struct {
	apiKey  string
	baseURL string
	model   string
}

func newAnthropicProvider(cfg Config) (*anthropicProvider, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("anthropic: Model is required")
	}

	base := cfg.BaseURL
	if base == "" {
		base = defaultAnthropicBaseURL
	}

	return &anthropicProvider{
		apiKey:  cfg.APIKey,
		baseURL: base,
		model:   cfg.Model,
	}, nil
}

func (a *anthropicProvider) Name() string { return "anthropic" }

func (a *anthropicProvider) Call(imageData []byte, prompt string) (string, error) {
	b64 := base64.StdEncoding.EncodeToString(imageData)

	reqBody := anthropicRequest{
		Model:     a.model,
		MaxTokens: defaultMaxTokens,
		Messages: []anthropicMessage{{
			Role: "user",
			Content: []anthropicContentBlock{
				{Type: "text", Text: prompt},
				{Type: "image", Source: &anthropicImageSource{
					Type:      "base64",
					MediaType: "image/jpeg",
					Data:      b64,
				}},
			},
		}},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	apiURL := a.baseURL + "/v1/messages"

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request: %w", err)
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	statusCode := resp.StatusCode
	retryAfter := resp.Header.Get("retry-after")
	resp.Body.Close()

	if statusCode == 429 {
		wait := parseRetryAfter(retryAfter, rpmWindow)
		log.Printf("captcha solver [anthropic]: rate limited, waiting %v", wait.Round(time.Second))
		return "", &RateLimitError{Wait: wait, Message: "anthropic: rate limited"}
	}

	if statusCode == 401 || statusCode == 403 {
		return "", &AuthError{Message: fmt.Sprintf("anthropic: auth failed (HTTP %d)", statusCode)}
	}

	if statusCode != 200 {
		var ae anthropicErrorResponse
		json.Unmarshal(body, &ae)
		return "", fmt.Errorf("anthropic: HTTP %d - %s", statusCode, truncate(ae.Error.Message, 100))
	}

	var gr anthropicResponse
	if err := json.Unmarshal(body, &gr); err != nil {
		return "", fmt.Errorf("anthropic: parse response: %w", err)
	}

	if gr.StopReason != "" && gr.StopReason != "end_turn" {
		return "", fmt.Errorf("anthropic: incomplete response: %s", gr.StopReason)
	}

	if len(gr.Content) == 0 || gr.Content[0].Text == "" {
		return "", fmt.Errorf("anthropic: empty response")
	}

	return gr.Content[0].Text, nil
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type   string               `json:"type"`
	Text   string               `json:"text,omitempty"`
	Source *anthropicImageSource `json:"source,omitempty"`
}

type anthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicResponse struct {
	Content    []anthropicResponseBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
}

type anthropicResponseBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}
