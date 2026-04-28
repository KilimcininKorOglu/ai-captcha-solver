package captcha

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func extractCode(text string) (string, error) {
	var cleaned strings.Builder
	for _, r := range strings.ToLower(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cleaned.WriteRune(r)
		}
	}

	code := cleaned.String()
	if len(code) < 4 || len(code) > 8 {
		return "", fmt.Errorf("invalid output: %q -> %q (%d chars)", truncate(text, 50), code, len(code))
	}

	return code, nil
}

func parseRetryAfter(header string, fallback time.Duration) time.Duration {
	if header == "" {
		return fallback
	}
	if seconds, err := strconv.Atoi(header); err == nil {
		if seconds <= 0 || seconds > int(fallback.Seconds()) {
			seconds = int(fallback.Seconds())
		}
		return time.Duration(seconds) * time.Second
	}
	if t, err := time.Parse(time.RFC1123, header); err == nil {
		if wait := time.Until(t); wait > 0 {
			if wait > fallback {
				return fallback
			}
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func buildURL(base, path string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, path) {
		return base
	}
	return base + path
}

func sanitizeKeyFromMessage(msg string) string {
	i := strings.Index(msg, "AIzaSy")
	if i == -1 {
		return msg
	}
	end := i
	for end < len(msg) && msg[end] != '\'' && msg[end] != '"' && msg[end] != ' ' && msg[end] != '.' {
		end++
	}
	if end-i > 10 {
		return msg[:i] + maskKey(msg[i:end]) + msg[end:]
	}
	return msg
}
