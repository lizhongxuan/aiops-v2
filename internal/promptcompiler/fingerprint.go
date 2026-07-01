package promptcompiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const promptFingerprintVersion = "prompt-fingerprint-v1"

// BuildPromptFingerprintForAdapter fingerprints a CompiledPrompt assembled by
// thin adapters that already provide a section-first envelope.
func BuildPromptFingerprintForAdapter(compiled CompiledPrompt) PromptFingerprint {
	return buildPromptFingerprint(compiled)
}

func buildPromptFingerprint(compiled CompiledPrompt) PromptFingerprint {
	return PromptFingerprint{
		Version:           promptFingerprintVersion,
		CompilerVersion:   promptFingerprintVersion,
		StableHash:        hashPromptText(CompiledPromptStableText(compiled)),
		SystemHash:        hashPromptText(CompiledPromptBaseContractText(compiled)),
		DeveloperHash:     hashPromptText(CompiledPromptProfileText(compiled)),
		ToolRegistryHash:  hashPromptText(CompiledPromptToolSurfaceText(compiled)),
		RuntimePolicyHash: hashPromptText(CompiledPromptRuntimeStateText(compiled)),
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
