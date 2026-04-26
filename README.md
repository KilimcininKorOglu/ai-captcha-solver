# gemini-captcha-solver

A Go library that solves image-based CAPTCHAs using the Google Gemini API. Supports API key pooling for high-throughput usage.

## Installation

```bash
go get github.com/KilimcininKorOglu/gemini-captcha-solver
```

## Usage

### Single Key

```go
solver := captcha.New(captcha.Config{
    APIKey: "your-gemini-api-key",
})

code, err := solver.Solve(imageData)
```

### Key Pool

```go
solver := captcha.New(captcha.Config{
    APIKeys: []string{
        "key-1",
        "key-2",
        "key-3",
    },
})

code, err := solver.Solve(imageData)
```

When a key hits rate limit (429), the solver automatically rotates to the next key. If all keys are exhausted, it waits using the `Retry-After` header (or exponential backoff) before retrying.

## Configuration

| Field      | Type            | Default                | Description                                |
|------------|-----------------|------------------------|--------------------------------------------|
| APIKey     | string          |                        | Single Gemini API key                      |
| APIKeys    | []string        |                        | Multiple API keys (pool, round-robin)      |
| Model      | string          | `gemini-2.5-flash`     | Gemini model name                          |
| Prompt     | string          | Generic CAPTCHA prompt | Custom prompt for CAPTCHA solving          |
| MaxRetries | int             | 5                      | Max retries on rate limit (429)            |
| Backoff    | time.Duration   | 15s                    | Initial backoff (exponential, max 2m)      |

Use `APIKey` for a single key, or `APIKeys` for a pool. If both are set, `APIKeys` takes priority.

## Rate Limiting

1. On HTTP 429, rotate to next key in pool (instant, no wait)
2. If all keys are rate limited, wait using `Retry-After` header from response
3. If no `Retry-After` header, use exponential backoff: 15s, 30s, 60s, 120s, 120s

## License

MIT
