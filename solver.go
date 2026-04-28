package captcha

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

const (
	defaultModel      = "gemini-2.5-flash-lite"
	defaultPrompt     = `Read the CAPTCHA text in this image.

RULES:
1. Output ONLY the characters (letters and digits) you see.
2. Do NOT describe the image.
3. Do NOT explain what a CAPTCHA is.
4. Do NOT add quotes, punctuation, or formatting.
5. Do NOT write sentences or words other than the CAPTCHA code.
6. The code is 4-8 characters long, mixed letters and digits.
7. Your entire response must be exactly the CAPTCHA code and nothing else.

GOOD examples: ab3xk, GPNTU, 7m2Kf
BAD examples: "The text says ab3xk", "CAPTCHA: ab3xk", "I can see the characters..."`
	defaultMaxRetries = 5
	defaultMaxTokens  = 256
	defaultDeadline   = 5 * time.Minute
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

type Config struct {
	Provider string
	BaseURL  string

	APIKey  string
	APIKeys []string

	Model      string
	Prompt     string
	MaxRetries int
}

type Solver struct {
	provider Provider
	cfg      Config
}

func New(cfg Config) *Solver {
	if cfg.Provider == "" {
		cfg.Provider = "gemini"
	}
	if cfg.Prompt == "" {
		cfg.Prompt = defaultPrompt
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = defaultMaxRetries
	}

	provider, err := newProvider(cfg)
	if err != nil {
		panic("captcha solver: " + err.Error())
	}

	return &Solver{
		cfg:      cfg,
		provider: provider,
	}
}

func Validate(cfg Config) error {
	if cfg.Provider == "" {
		cfg.Provider = "gemini"
	}

	if cfg.APIKey == "" && len(cfg.APIKeys) == 0 {
		return fmt.Errorf("APIKey or APIKeys required")
	}

	if cfg.Provider != "gemini" && len(cfg.APIKeys) > 0 {
		return fmt.Errorf("APIKeys (key pool) is only supported for the gemini provider")
	}

	if cfg.Provider != "gemini" && cfg.Model == "" {
		return fmt.Errorf("Model is required for %s provider", cfg.Provider)
	}

	return nil
}

func (s *Solver) Solve(imageData []byte) (string, error) {
	deadline := time.Now().Add(defaultDeadline)

	for attempt := 0; attempt < s.cfg.MaxRetries; {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return "", fmt.Errorf("deadline exceeded after %d attempts", attempt)
		}

		rawText, err := s.provider.Call(imageData, s.cfg.Prompt)
		if err != nil {
			var rle *RateLimitError
			if errors.As(err, &rle) {
				if wait, ok := rle.Wait.(time.Duration); ok && wait > 0 {
					if wait > remaining {
						return "", fmt.Errorf("rate limited for %v, only %v until deadline", wait, remaining)
					}
					log.Printf("captcha solver [%s]: %s, waiting %v", s.provider.Name(), rle.Message, wait.Round(time.Second))
					time.Sleep(wait)
				}
				continue
			}

			var ae *AuthError
			if errors.As(err, &ae) {
				if s.provider.Name() != "gemini" {
					return "", fmt.Errorf("%s: %w", s.provider.Name(), ae)
				}
				continue
			}

			attempt++
			if attempt < s.cfg.MaxRetries {
				continue
			}
			return "", err
		}

		code, err := extractCode(rawText)
		if err != nil {
			attempt++
			if attempt < s.cfg.MaxRetries {
				continue
			}
			return "", err
		}

		return code, nil
	}

	return "", fmt.Errorf("max retries (%d) exceeded", s.cfg.MaxRetries)
}
