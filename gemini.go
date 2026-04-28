package captcha

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	defaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	rpmWindow            = 60 * time.Second
)

type geminiProvider struct {
	mu       sync.Mutex
	keys     []string
	current  int
	cooldown map[string]time.Time
	baseURL  string
	model    string
}

func newGeminiProvider(cfg Config) *geminiProvider {
	keys := cfg.APIKeys
	if len(keys) == 0 && cfg.APIKey != "" {
		keys = []string{cfg.APIKey}
	}

	base := cfg.BaseURL
	if base == "" {
		base = defaultGeminiBaseURL
	}

	model := cfg.Model
	if model == "" {
		model = defaultModel
	}

	if info, ok := Models[model]; ok && info.Deprecated {
		log.Printf("captcha solver: WARNING: model %s is deprecated, consider switching to %s", model, defaultModel)
	}

	return &geminiProvider{
		keys:     keys,
		cooldown: make(map[string]time.Time),
		baseURL:  base,
		model:    model,
	}
}

func (g *geminiProvider) Name() string { return "gemini" }

func (g *geminiProvider) acquireKey() (string, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.keys) == 0 {
		return "", 0
	}

	now := time.Now()
	var earliest time.Time

	for i := 0; i < len(g.keys); i++ {
		idx := (g.current + i) % len(g.keys)
		key := g.keys[idx]
		if exp, ok := g.cooldown[key]; ok && now.Before(exp) {
			if earliest.IsZero() || exp.Before(earliest) {
				earliest = exp
			}
			continue
		}
		delete(g.cooldown, key)
		g.current = idx + 1
		return key, 0
	}

	return "", time.Until(earliest)
}

func (g *geminiProvider) markCooldown(key string, d time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cooldown[key] = time.Now().Add(d)
}

func (g *geminiProvider) Call(imageData []byte, prompt string) (string, error) {
	b64 := base64.StdEncoding.EncodeToString(imageData)

	reqBody := geminiRequest{
		Contents: []geminiContent{{
			Parts: []geminiPart{
				{Text: prompt},
				{InlineData: &geminiInlineData{MimeType: "image/jpeg", Data: b64}},
			},
		}},
		GenerationConfig: geminiGenerationConfig{
			Temperature:     0,
			MaxOutputTokens: defaultMaxTokens,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	key, wait := g.acquireKey()
	if key == "" && wait > 0 {
		return "", &RateLimitError{Wait: wait, Message: fmt.Sprintf("all keys cooling down for %v", wait.Round(time.Second))}
	}
	if key == "" {
		return "", fmt.Errorf("no API keys configured")
	}

	apiURL := fmt.Sprintf("%s/models/%s:generateContent", g.baseURL, g.model)

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", key)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request: %w", err)
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	statusCode := resp.StatusCode
	retryAfter := resp.Header.Get("Retry-After")
	resp.Body.Close()

	if statusCode == 429 {
		cooldown := parseRetryAfter(retryAfter, rpmWindow)
		masked := maskKey(key)
		log.Printf("captcha solver: key %s rate limited, cooling down %v", masked, cooldown.Round(time.Second))
		g.markCooldown(key, cooldown)
		return "", &RateLimitError{Wait: cooldown, Message: fmt.Sprintf("key %s rate limited", masked)}
	}

	if statusCode == 401 || statusCode == 403 {
		masked := maskKey(key)
		log.Printf("captcha solver: key %s auth failed (HTTP %d), permanently disabled", masked, statusCode)
		g.markCooldown(key, 24*time.Hour)
		return "", &RateLimitError{Wait: 24 * time.Hour, Message: fmt.Sprintf("key %s auth failed", masked)}
	}

	if statusCode != 200 {
		var ge geminiError
		json.Unmarshal(body, &ge)
		return "", fmt.Errorf("API error: HTTP %d - %s", statusCode, truncate(sanitizeKeyFromMessage(ge.Error.Message), 100))
	}

	return geminiExtractText(body)
}

func geminiExtractText(body []byte) (string, error) {
	var gr geminiResponse
	if err := json.Unmarshal(body, &gr); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if gr.PromptFeedback != nil && gr.PromptFeedback.BlockReason != "" {
		return "", fmt.Errorf("safety filter: %s", gr.PromptFeedback.BlockReason)
	}

	if len(gr.Candidates) == 0 {
		return "", fmt.Errorf("empty response")
	}

	candidate := gr.Candidates[0]
	if candidate.FinishReason != "" && candidate.FinishReason != "STOP" {
		return "", fmt.Errorf("incomplete response: %s", candidate.FinishReason)
	}

	if len(candidate.Content.Parts) == 0 || candidate.Content.Parts[0].Text == "" {
		return "", fmt.Errorf("no text in response")
	}

	return candidate.Content.Parts[0].Text, nil
}
