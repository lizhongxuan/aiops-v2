package promptcompiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const promptFingerprintVersion = "prompt-fingerprint-v1"

func buildPromptFingerprint(compiled CompiledPrompt) PromptFingerprint {
	return PromptFingerprint{
		Version:           promptFingerprintVersion,
		CompilerVersion:   promptFingerprintVersion,
		StableHash:        hashPromptText(compiled.Stable.Content),
		SystemHash:        hashPromptText(compiled.Stable.System.Content),
		DeveloperHash:     hashPromptText(compiled.Stable.Developer.Content),
		ToolRegistryHash:  hashPromptText(compiled.Stable.Tools.Content),
		RuntimePolicyHash: hashPromptText(compiled.Dynamic.Policy.Content),
		ProtocolStateHash: hashPromptJSON(compiled.Dynamic.ProtocolState),
	}
}

func hashPromptText(value string) string {
	return hashPromptBytes([]byte(strings.TrimSpace(value)))
}

func hashPromptJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return hashPromptText("")
	}
	return hashPromptBytes(data)
}

func hashPromptBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
