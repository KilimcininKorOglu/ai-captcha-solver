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
	defaultBackoff    = 6 * time.Second
	defaultMaxTokens  = 256
	defaultDeadline   = 5 * time.Minute

	geminiAPIURL = "https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent"
)

type Config struct {
	APIKey     string
	APIKeys    []string
	Model      string
	Prompt     string
	MaxRetries int
	Backoff    time.Duration
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
	if cfg.Prompt == "" {
		cfg.Prompt = defaultPrompt
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = defaultMaxRetries
	}
	if cfg.Backoff <= 0 {
		cfg.Backoff = defaultBackoff
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
	var earliestExpiry time.Time

	for i := 0; i < len(s.keys); i++ {
		idx := (s.current + i) % len(s.keys)
		key := s.keys[idx]
		if expiry, limited := s.cooldown[key]; limited && now.Before(expiry) {
			if earliestExpiry.IsZero() || expiry.Before(earliestExpiry) {
				earliestExpiry = expiry
			}
			continue
		}
		delete(s.cooldown, key)
		s.current = idx + 1
		return key, 0
	}

	return "", time.Until(earliestExpiry)
}

func (s *Solver) markRateLimited(key string, wait time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cooldown[key] = time.Now().Add(wait)
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
	rateLimitHits := 0

	for attempt := 0; attempt < s.cfg.MaxRetries; {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("deadline exceeded after %d attempts (%d rate limits)", attempt, rateLimitHits)
		}

		key, wait := s.acquireKey()
		if key == "" && wait > 0 {
			if wait > 2*time.Minute {
				wait = 2 * time.Minute
			}
			log.Printf("captcha solver: all keys cooling down, waiting %v", wait)
			time.Sleep(wait)
			key, wait = s.acquireKey()
			if key == "" {
				return "", fmt.Errorf("no available keys after cooldown wait")
			}
		} else if key == "" {
			return "", fmt.Errorf("no API keys configured")
		}

		req, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonBody))
		if err != nil {
			return "", fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", key)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("API request: %w", err)
		}

		body, _ := io.ReadAll(resp.Body)
		statusCode := resp.StatusCode
		resp.Body.Close()

		if statusCode == 429 {
			rateLimitHits++
			retryWait := parseRetryAfter(resp.Header.Get("Retry-After"), s.cfg.Backoff)
			maskedKey := key[:8] + "..." + key[len(key)-4:]
			log.Printf("captcha solver: key %s rate limited, cooldown %v (rate limits: %d)", maskedKey, retryWait, rateLimitHits)
			s.markRateLimited(key, retryWait)
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

	return "", fmt.Errorf("rate limit: all keys exhausted after %d retries", s.cfg.MaxRetries)
}

func parseRetryAfter(header string, fallback time.Duration) time.Duration {
	if header == "" {
		return fallback
	}
	if seconds, err := strconv.Atoi(header); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if t, err := time.Parse(time.RFC1123, header); err == nil {
		wait := time.Until(t)
		if wait > 0 {
			return wait
		}
	}
	return fallback
}
