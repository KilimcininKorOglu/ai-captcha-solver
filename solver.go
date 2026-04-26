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
	defaultModel      = "gemini-2.5-flash"
	defaultPrompt     = "Read the CAPTCHA text. Reply with ONLY the characters (letters and numbers), nothing else."
	defaultMaxRetries = 5
	defaultBackoff    = 15 * time.Second
	defaultMaxTokens  = 256

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
	mu      sync.Mutex
	keys    []string
	current int
	cfg     Config
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

	return &Solver{cfg: cfg, keys: keys}
}

func (s *Solver) nextKey() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.keys) == 0 {
		return ""
	}
	key := s.keys[s.current%len(s.keys)]
	s.current++
	return key
}

func (s *Solver) rotateKey() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.keys) == 0 {
		return ""
	}
	s.current++
	return s.keys[s.current%len(s.keys)]
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
	currentKey := s.nextKey()
	if currentKey == "" {
		return "", fmt.Errorf("no API keys configured")
	}

	var body []byte
	var statusCode int
	keysExhausted := 0

	for attempt := 0; attempt < s.cfg.MaxRetries; attempt++ {
		req, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonBody))
		if err != nil {
			return "", fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", currentKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("API request: %w", err)
		}

		body, _ = io.ReadAll(resp.Body)
		statusCode = resp.StatusCode
		resp.Body.Close()

		if statusCode == 429 {
			if len(s.keys) > 1 {
				nextKey := s.rotateKey()
				if nextKey != currentKey {
					log.Printf("captcha solver: key rate limited, rotating to next key (attempt %d/%d)", attempt+1, s.cfg.MaxRetries)
					currentKey = nextKey
					keysExhausted = 0
					continue
				}
				keysExhausted++
			}

			if keysExhausted >= len(s.keys) || len(s.keys) <= 1 {
				wait := parseRetryAfter(resp.Header.Get("Retry-After"), s.cfg.Backoff*time.Duration(1<<attempt))
				if wait > 2*time.Minute {
					wait = 2 * time.Minute
				}
				log.Printf("captcha solver: all keys rate limited, waiting %v (attempt %d/%d)", wait, attempt+1, s.cfg.MaxRetries)
				time.Sleep(wait)
				keysExhausted = 0
				currentKey = s.nextKey()
			}
			continue
		}
		break
	}

	if statusCode != 200 {
		var ge geminiError
		json.Unmarshal(body, &ge)
		switch statusCode {
		case 429:
			return "", fmt.Errorf("rate limit: all keys exhausted after %d retries", s.cfg.MaxRetries)
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
