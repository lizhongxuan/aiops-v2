package taskdepth

import "strings"

type Level string

const (
	LevelTrivial       Level = "trivial"
	LevelSimpleRead    Level = "simple_read"
	LevelMultiStep     Level = "multi_step"
	LevelInvestigation Level = "investigation"
	LevelOperations    Level = "operations"
	LevelMultiAgent    Level = "multi_agent"
)

type Profile struct {
	Level                Level    `json:"level,omitempty"`
	Reasons              []string `json:"reasons,omitempty"`
	RequiresPlan         bool     `json:"requiresPlan,omitempty"`
	RequiresEvidence     bool     `json:"requiresEvidence,omitempty"`
	RequiresValidation   bool     `json:"requiresValidation,omitempty"`
	AllowsFirstTurnFinal bool     `json:"allowsFirstTurnFinal,omitempty"`
}

type Options struct {
	Input    string
	Mode     string
	Metadata map[string]string
}

func NormalizeLevel(value string) Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(LevelSimpleRead), "simple-read", "simple":
		return LevelSimpleRead
	case string(LevelMultiStep), "multi-step", "multistep":
		return LevelMultiStep
	case string(LevelInvestigation), "rca", "incident":
		return LevelInvestigation
	case string(LevelOperations), "operation", "mutation", "execute":
		return LevelOperations
	case string(LevelMultiAgent), "multi-agent", "multi_host", "multi-host":
		return LevelMultiAgent
	case string(LevelTrivial), "":
		return LevelTrivial
	default:
		return LevelTrivial
	}
}

func Rank(level Level) int {
	switch NormalizeLevel(string(level)) {
	case LevelTrivial:
		return 0
	case LevelSimpleRead:
		return 1
	case LevelMultiStep:
		return 2
	case LevelInvestigation:
		return 3
	case LevelOperations:
		return 4
	case LevelMultiAgent:
		return 5
	default:
		return 0
	}
}

func AtLeast(level, minimum Level) bool {
	return Rank(level) >= Rank(minimum)
}
