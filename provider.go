package captcha

import "fmt"

type Provider interface {
	Call(imageData []byte, prompt string) (string, error)
	Name() string
}

type RateLimitError struct {
	Wait    interface{}
	Message string
}

func (e *RateLimitError) Error() string { return e.Message }

type AuthError struct {
	Message string
}

func (e *AuthError) Error() string { return e.Message }

func newProvider(cfg Config) (Provider, error) {
	switch cfg.Provider {
	case "gemini", "":
		return newGeminiProvider(cfg), nil
	case "openai":
		return newOpenAIProvider(cfg)
	case "anthropic":
		return newAnthropicProvider(cfg)
	default:
		return nil, fmt.Errorf("unknown provider: %q (supported: gemini, openai, anthropic)", cfg.Provider)
	}
}
