package selfopt

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

func LoadConfig(opts Options) Config {
	cfg := Config{
		ServerURL:       firstNonEmpty(opts.ServerURL, "http://127.0.0.1:8080"),
		AllowRealLLM:    opts.AllowRealLLM,
		AllowRemoteHost: opts.AllowRemoteHost,
		ServerLLM:       loadLLM("AIOPS_LLM", "server", opts.AllowRealLLM),
		LabLLM:          loadLLM("AIOPS_LAB_LLM", "lab", opts.LLMSuggestions),
	}
	return cfg
}

func loadLLM(prefix, source string, allow bool) LLMConfig {
	baseURL := os.Getenv(prefix + "_BASE_URL")
	model := os.Getenv(prefix + "_MODEL")
	apiKey := os.Getenv(prefix + "_API_KEY")
	enabled := allow && baseURL != "" && model != "" && apiKey != ""
	return LLMConfig{
		Enabled:          enabled,
		BaseURL:          baseURL,
		BaseURLHash:      hashString(baseURL),
		Model:            model,
		APIKeyConfigured: apiKey != "",
		Source:           source,
	}
}

func NewManifest(runID string, cfg Config, cases []Case) Manifest {
	ids := make([]string, 0, len(cases))
	for _, c := range cases {
		ids = append(ids, c.ID)
	}
	cfg.ServerLLM.BaseURL = ""
	cfg.LabLLM.BaseURL = ""
	return Manifest{
		RunID:     runID,
		StartedAt: nowUTC(),
		Config:    cfg,
		Cases:     ids,
		Safety:    "offline_by_default_no_prod_mutation",
	}
}

func hashString(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
