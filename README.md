# gemini-captcha-solver

A Go library that solves image-based CAPTCHAs using the Google Gemini API.

## Installation

```bash
go get github.com/KilimcininKorOglu/gemini-captcha-solver
```

## Usage

```go
package main

import (
    "fmt"
    "os"

    captcha "github.com/KilimcininKorOglu/gemini-captcha-solver"
)

func main() {
    solver := captcha.New(captcha.Config{
        APIKey: "your-gemini-api-key",
        Model:  "gemini-2.5-flash",
    })

    imageData, _ := os.ReadFile("captcha.jpg")

    code, err := solver.Solve(imageData)
    if err != nil {
        fmt.Fprintf(os.Stderr, "solve failed: %v\n", err)
        os.Exit(1)
    }
    fmt.Println("CAPTCHA:", code)
}
```

## Configuration

| Field      | Type            | Default               | Description                         |
|------------|-----------------|-----------------------|-------------------------------------|
| APIKey     | string          | (required)            | Google Gemini API key               |
| Model      | string          | `gemini-2.5-flash`    | Gemini model name                   |
| Prompt     | string          | Generic CAPTCHA prompt | Custom prompt for CAPTCHA solving  |
| MaxRetries | int             | 5                     | Max retries on rate limit (429)     |
| Backoff    | time.Duration   | 15s                   | Initial backoff (exponential, max 2m) |

## Rate Limiting

Automatically handles HTTP 429 responses with exponential backoff: 15s, 30s, 60s, 120s, 120s.

## License

MIT
