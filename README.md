# ai-captcha-solver

A Go library that solves image-based CAPTCHAs using AI vision APIs. Supports Gemini, OpenAI, and Anthropic providers with API key pooling for Gemini.

## Installation

```bash
go get github.com/KilimcininKorOglu/ai-captcha-solver
```

Requires Go 1.26 or later.

## Quick Start

### Gemini (default)

```go
solver := captcha.New(captcha.Config{
    APIKey: os.Getenv("GEMINI_API_KEY"),
})

code, err := solver.Solve(imageData)
```

### OpenAI

```go
solver := captcha.New(captcha.Config{
    Provider: "openai",
    APIKey:   os.Getenv("OPENAI_API_KEY"),
    Model:    "gpt-4o-mini",
})

code, err := solver.Solve(imageData)
```

### Anthropic

```go
solver := captcha.New(captcha.Config{
    Provider: "anthropic",
    APIKey:   os.Getenv("ANTHROPIC_API_KEY"),
    Model:    "claude-sonnet-4-20250514",
})

code, err := solver.Solve(imageData)
```

### Custom Base URL

All providers support custom base URLs for proxies or self-hosted endpoints:

```go
solver := captcha.New(captcha.Config{
    Provider: "openai",
    BaseURL:  "https://my-proxy.example.com",
    APIKey:   os.Getenv("API_KEY"),
    Model:    "gpt-4o-mini",
})
```

## API Key Pool (Gemini only)

Distribute requests across multiple API keys to avoid rate limits. Keys rotate round-robin. On HTTP 429, the rate-limited key enters a cooldown period while the remaining keys continue serving requests. If all keys are cooling down, the solver waits for the earliest one to become available.

```go
solver := captcha.New(captcha.Config{
    APIKeys: []string{
        os.Getenv("GEMINI_KEY_1"),
        os.Getenv("GEMINI_KEY_2"),
        os.Getenv("GEMINI_KEY_3"),
    },
})
```

The solver is safe for concurrent use from multiple goroutines.

Key pooling is only available for the Gemini provider. OpenAI and Anthropic use a single key.

## Configuration

| Field      | Type     | Default                 | Description                                                        |
|------------|----------|-------------------------|--------------------------------------------------------------------|
| Provider   | string   | `gemini`                | AI provider: `gemini`, `openai`, `anthropic`                       |
| BaseURL    | string   | Provider default        | Custom API base URL                                                |
| APIKey     | string   |                         | Single API key                                                     |
| APIKeys    | []string |                         | Key pool (Gemini only, round-robin, takes priority over APIKey)    |
| Model      | string   | `gemini-2.5-flash-lite` | Model name (required for OpenAI and Anthropic, optional for Gemini)|
| Prompt     | string   | Generic CAPTCHA prompt  | Custom prompt sent to the AI                                       |
| MaxRetries | int      | 5                       | Max attempts for non-rate-limit errors (429 retries are unlimited) |

## Provider Defaults

| Provider  | Default Base URL                                              | Default Model           | Key Pool |
|-----------|---------------------------------------------------------------|-------------------------|----------|
| gemini    | `https://generativelanguage.googleapis.com/v1beta`            | `gemini-2.5-flash-lite` | Yes      |
| openai    | `https://api.openai.com`                                      | (required)              | No       |
| anthropic | `https://api.anthropic.com`                                   | (required)              | No       |

## Rate Limit Handling

1. On HTTP 429, the solver pauses using the `Retry-After` response header; if absent, falls back to 60 seconds
2. For Gemini with key pool: the rate-limited key enters per-key cooldown, other keys remain available
3. For OpenAI/Anthropic: the solver sleeps for the retry duration before retrying
4. On HTTP 401/403: Gemini disables the key for 24 hours; OpenAI/Anthropic return a fatal error
5. A hard deadline of 5 minutes caps total `Solve` wall time regardless of retries

## How It Works

1. Base64-encodes the CAPTCHA image
2. Sends it to the configured AI API with a text prompt asking to read the characters
3. Parses the response, lowercases the text, strips non-alphanumeric characters, validates length (4-8 chars)
4. Returns the cleaned lowercase CAPTCHA text

## Free API Providers

### Gemini (Google)

Free tier with rate limits. Get an API key at [Google AI Studio](https://aistudio.google.com/app/apikey).

### NVIDIA NIM (HermesTech)

Free access to multiple vision models via the Anthropic-compatible API at [HermesTech](https://nvidia.srv.hermestech.uk). No credit card required.

```go
solver := captcha.New(captcha.Config{
    Provider: "anthropic",
    BaseURL:  "https://nvidia.srv.hermestech.uk/v1/messages",
    APIKey:   "your-hermes-api-key",
    Model:    "microsoft/phi-4-multimodal-instruct",
})
```

Available vision models (tested with CAPTCHA solving):

| Model                                     | Speed  | CAPTCHA |
|-------------------------------------------|--------|---------|
| `microsoft/phi-4-multimodal-instruct`     | ~400ms | Best    |
| `nvidia/nemotron-nano-12b-v2-vl`          | ~600ms | Best    |
| `qwen/qwen3.5-122b-a10b`                 | ~700ms | Best    |
| `meta/llama-4-maverick-17b-128e-instruct` | ~2s    | Good    |
| `meta/llama-3.2-11b-vision-instruct`      | ~4s    | Good    |
| `google/gemma-3n-e2b-it`                  | ~6s    | Good    |
| `google/gemma-3n-e4b-it`                  | ~9s    | Good    |
| `meta/llama-3.2-90b-vision-instruct`      | ~9s    | Good    |

### Paid API Keys

- OpenAI: [OpenAI Platform](https://platform.openai.com/api-keys)
- Anthropic: [Anthropic Console](https://console.anthropic.com/settings/keys)

## License

MIT
