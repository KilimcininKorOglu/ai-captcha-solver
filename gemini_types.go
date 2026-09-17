package captcha

type geminiRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inline_data,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type geminiGenerationConfig struct {
	Temperature     float64 `json:"temperature"`
	MaxOutputTokens int     `json:"maxOutputTokens"`
}

type geminiResponse struct {
	Candidates     []geminiCandidate     `json:"candidates"`
	PromptFeedback *geminiPromptFeedback `json:"promptFeedback,omitempty"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type geminiPromptFeedback struct {
	BlockReason string `json:"blockReason,omitempty"`
}

type geminiError struct {
	Error struct {
		Message string              `json:"message"`
		Details []geminiErrorDetail `json:"details"`
	} `json:"error"`
}

// geminiErrorDetail is one entry of the google.rpc error details. A 429 carries
// a QuotaFailure naming the exhausted quota and a RetryInfo suggesting a delay.
type geminiErrorDetail struct {
	Type       string                 `json:"@type"`
	Violations []geminiQuotaViolation `json:"violations"`
	RetryDelay string                 `json:"retryDelay"`
}

// geminiQuotaViolation names one exhausted quota. QuotaID tells the window
// apart: a value with "PerDay" does not refill until the daily reset, while
// "PerMinute" refills within the minute.
type geminiQuotaViolation struct {
	QuotaID    string `json:"quotaId"`
	QuotaValue string `json:"quotaValue"`
}
