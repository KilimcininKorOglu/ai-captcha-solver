package captcha

type ModelInfo struct {
	ID         string
	RPM        int
	RPD        int
	TPM        int
	Deprecated bool
}

var Models = map[string]ModelInfo{
	"gemini-2.5-flash-lite": {
		ID: "gemini-2.5-flash-lite", RPM: 15, RPD: 1000, TPM: 250000,
	},
	"gemini-2.5-flash": {
		ID: "gemini-2.5-flash", RPM: 10, RPD: 250, TPM: 250000,
	},
	"gemini-2.5-pro": {
		ID: "gemini-2.5-pro", RPM: 5, RPD: 100, TPM: 250000,
	},
	"gemini-3.1-flash-lite-preview": {
		ID: "gemini-3.1-flash-lite-preview", RPM: 15, RPD: 1000, TPM: 250000,
	},
	"gemini-3-flash-preview": {
		ID: "gemini-3-flash-preview", RPM: 10, RPD: 250, TPM: 250000,
	},
	"gemini-3.1-pro-preview": {
		ID: "gemini-3.1-pro-preview", RPM: 5, RPD: 100, TPM: 250000,
	},
	"gemini-2.0-flash": {
		ID: "gemini-2.0-flash", RPM: 15, RPD: 1500, TPM: 250000, Deprecated: true,
	},
	"gemini-2.0-flash-lite": {
		ID: "gemini-2.0-flash-lite", RPM: 15, RPD: 1500, TPM: 250000, Deprecated: true,
	},
}

func ActiveModels() []ModelInfo {
	var active []ModelInfo
	for _, m := range Models {
		if !m.Deprecated {
			active = append(active, m)
		}
	}
	return active
}
