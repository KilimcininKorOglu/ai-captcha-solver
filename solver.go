package captcha

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultModel      = "gemini-2.5-flash-lite"
	defaultPrompt     = "Read the CAPTCHA text. Reply with ONLY the characters (letters and numbers), nothing else."
	defaultMaxRetries = 5
	defaultMaxTokens  = 256
	defaultDeadline   = 5 * time.Minute
	rpmWindow         = 60 * time.Second

	geminiAPIURL = "https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

type Config struct {
	APIKey     string
	APIKeys    []string
	Model      string
	Prompt     string
	MaxRetries int
}

type Solver struct {
	mu       sync.Mutex
	keys     []string
	current  int
	cooldown map[string]time.Time
	cfg      Config
}

func New(cfg Config) *Solver {
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	if info, ok := Models[cfg.Model]; ok && info.Deprecated {
		log.Printf("captcha solver: WARNING: model %s is deprecated, consider switching to %s", cfg.Model, defaultModel)
	}
	if cfg.Prompt == "" {
		cfg.Prompt = defaultPrompt
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = defaultMaxRetries
	}

	keys := cfg.APIKeys
	if len(keys) == 0 && cfg.APIKey != "" {
		keys = []string{cfg.APIKey}
	}

	return &Solver{
		cfg:      cfg,
		keys:     keys,
		cooldown: make(map[string]time.Time),
	}
}

func (s *Solver) acquireKey() (string, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.keys) == 0 {
		return "", 0
	}

	now := time.Now()
	var earliest time.Time

	for i := 0; i < len(s.keys); i++ {
		idx := (s.current + i) % len(s.keys)
		key := s.keys[idx]
		if exp, ok := s.cooldown[key]; ok && now.Before(exp) {
			if earliest.IsZero() || exp.Before(earliest) {
				earliest = exp
			}
			continue
		}
		delete(s.cooldown, key)
		s.current = idx + 1
		return key, 0
	}

	return "", time.Until(earliest)
}

func (s *Solver) markCooldown(key string, d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cooldown[key] = time.Now().Add(d)
}

func (s *Solver) Solve(imageData []byte) (string, error) {
	b64 := base64.StdEncoding.EncodeToString(imageData)

	reqBody := geminiRequest{
		Contents: []geminiContent{{
			Parts: []geminiPart{
				{Text: s.cfg.Prompt},
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

	apiURL := fmt.Sprintf(geminiAPIURL, s.cfg.Model)
	deadline := time.Now().Add(defaultDeadline)

	for attempt := 0; attempt < s.cfg.MaxRetries; {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return "", fmt.Errorf("deadline exceeded after %d attempts", attempt)
		}

		key, wait := s.acquireKey()

		if key == "" && wait > 0 {
			if wait > remaining {
				return "", fmt.Errorf("all keys cooling down for %v, only %v until deadline", wait, remaining)
			}
			log.Printf("captcha solver: all keys cooling down, waiting %v", wait.Round(time.Second))
			time.Sleep(wait)
			continue
		}
		if key == "" {
			return "", fmt.Errorf("no API keys configured")
		}

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
			s.markCooldown(key, cooldown)
			continue
		}

		attempt++

		if statusCode != 200 {
			var ge geminiError
			json.Unmarshal(body, &ge)
			switch statusCode {
			case 401, 403:
				return "", fmt.Errorf("auth error: %s", ge.Error.Message)
			default:
				return "", fmt.Errorf("API error: HTTP %d - %s", statusCode, ge.Error.Message)
			}
		}

		return extractText(body)
	}

	return "", fmt.Errorf("max retries (%d) exceeded", s.cfg.MaxRetries)
}

func extractText(body []byte) (string, error) {
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

	text := candidate.Content.Parts[0].Text
	var cleaned strings.Builder
	for _, r := range strings.ToLower(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cleaned.WriteRune(r)
		}
	}

	code := cleaned.String()
	if len(code) < 4 || len(code) > 8 {
		return "", fmt.Errorf("invalid output: %q -> %q (%d chars)", text, code, len(code))
	}

	return code, nil
}

func parseRetryAfter(header string, fallback time.Duration) time.Duration {
	if header == "" {
		return fallback
	}
	if seconds, err := strconv.Atoi(header); err == nil {
		if seconds > int(fallback.Seconds()) {
			seconds = int(fallback.Seconds())
		}
		return time.Duration(seconds) * time.Second
	}
	if t, err := time.Parse(time.RFC1123, header); err == nil {
		if wait := time.Until(t); wait > 0 {
			return wait
		}
	}
	return fallback
}

func maskKey(key string) string {
	if len(key) < 12 {
		return "***"
	}
	return key[:8] + "..." + key[len(key)-4:]
}
