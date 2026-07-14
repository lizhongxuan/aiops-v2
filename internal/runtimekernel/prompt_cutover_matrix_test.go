package runtimekernel

import (
	"testing"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
)

func TestPromptCutoverMatrixRuntimeContractVersionChange(t *testing.T) {
	baseCompiled, err := promptcompiler.NewCompiler().Compile(promptcompiler.CompileContext{
		SessionType: "host",
		Mode:        "inspect",
		HostContext: "synthetic-host",
	})
	if err != nil {
		t.Fatalf("Compile(base) error = %v", err)
	}
	versionChanged := baseCompiled
	versionChanged.EnvelopeV2.Sections = append([]promptcompiler.PromptCompiledSection(nil), baseCompiled.EnvelopeV2.Sections...)
	foundContract := false
	for index := range versionChanged.EnvelopeV2.Sections {
		if versionChanged.EnvelopeV2.Sections[index].LogicalLayer != promptcompiler.LayerStableRuntimeContract {
			continue
		}
		versionChanged.EnvelopeV2.Sections[index].Content += "\nruntime_contract_version: aiops.runtime.v-next"
		foundContract = true
		break
	}
	if !foundContract {
		t.Fatal("compiled prompt is missing the L2 runtime contract")
	}
	if err := versionChanged.EnvelopeV2.Validate(); err != nil {
		t.Fatalf("version-changed EnvelopeV2.Validate() error = %v", err)
	}

	history := []Message{{Role: "user", Content: "inspect the synthetic host"}}
	buildFingerprint := func(name string, compiled promptcompiler.CompiledPrompt) promptcompiler.PromptFingerprint {
		t.Helper()
		result, err := buildRuntimePromptInputV2WithContextGovernance(history, compiled, nil, 0, nil)
		if err != nil {
			t.Fatalf("%s runtime V2 build error = %v", name, err)
		}
		fingerprint, err := promptinput.BuildModelInputPromptFingerprint(result.Items)
		if err != nil {
			t.Fatalf("%s model-input fingerprint error = %v", name, err)
		}
		return fingerprint
	}
	base := buildFingerprint("base", baseCompiled)
	changed := buildFingerprint("contract-version-changed", versionChanged)

	if base.StableRuntimeContractHash == changed.StableRuntimeContractHash ||
		base.StablePrefixHash == changed.StablePrefixHash ||
		base.ModelInputHash == changed.ModelInputHash {
		t.Fatalf("runtime contract version did not change L2/stable/model hashes: base=%#v changed=%#v", base, changed)
	}
	if base.AbsoluteSystemHash != changed.AbsoluteSystemHash || base.RoleProfileHash != changed.RoleProfileHash {
		t.Fatalf("runtime contract version polluted L0/L1: base=%#v changed=%#v", base, changed)
	}
}

func assertPromptCutoverStableL0L3(t *testing.T, before, after map[string]string) {
	t.Helper()
	for _, key := range []string{"absoluteSystemHash", "roleProfileHash", "stableRuntimeContractHash", "turnStableHash"} {
		if before[key] == "" || after[key] == "" {
			t.Fatalf("prompt fingerprint is missing %s: before=%#v after=%#v", key, before, after)
		}
		if before[key] != after[key] {
			t.Fatalf("dynamic step change polluted %s: before=%q after=%q", key, before[key], after[key])
		}
	}
}

func assertPromptCutoverHashesChanged(t *testing.T, before, after map[string]string, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if before[key] == after[key] {
			t.Fatalf("prompt fingerprint %s did not change: %q", key, before[key])
		}
	}
}
