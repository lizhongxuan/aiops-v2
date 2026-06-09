package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

type ServerInstruction struct {
	ServerID  string    `json:"serverId"`
	Text      string    `json:"text"`
	Hash      string    `json:"hash,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

type AnnouncedMCPInstruction struct {
	ServerID string `json:"serverId"`
	Hash     string `json:"hash,omitempty"`
	Summary  string `json:"summary,omitempty"`
}

type MCPInstructionDelta struct {
	ServerID string `json:"serverId"`
	Action   string `json:"action"` // added | changed | removed
	Text     string `json:"text,omitempty"`
	Hash     string `json:"hash,omitempty"`
	Chars    int    `json:"chars,omitempty"`
	Summary  string `json:"summary,omitempty"`
}

type MCPInstructionSessionState struct {
	Announced map[string]AnnouncedMCPInstruction `json:"announced,omitempty"`
	LastDelta []MCPInstructionDelta              `json:"lastDelta,omitempty"`
}

func (s *MCPInstructionSessionState) Apply(deltas []MCPInstructionDelta) {
	if s == nil {
		return
	}
	if s.Announced == nil {
		s.Announced = map[string]AnnouncedMCPInstruction{}
	}
	s.LastDelta = append([]MCPInstructionDelta(nil), deltas...)
	for _, delta := range deltas {
		serverID := strings.TrimSpace(delta.ServerID)
		if serverID == "" {
			continue
		}
		if delta.Action == "removed" {
			delete(s.Announced, serverID)
			continue
		}
		s.Announced[serverID] = AnnouncedMCPInstruction{
			ServerID: serverID,
			Hash:     delta.Hash,
			Summary:  delta.Summary,
		}
	}
}

func buildInstructionDelta(instructions []ServerInstruction, disabled map[string]bool, state *MCPInstructionSessionState) []MCPInstructionDelta {
	announced := map[string]AnnouncedMCPInstruction{}
	if state != nil {
		for key, value := range state.Announced {
			announced[key] = value
		}
	}
	var deltas []MCPInstructionDelta
	current := map[string]ServerInstruction{}
	for _, instruction := range instructions {
		current[instruction.ServerID] = instruction
		if disabled[instruction.ServerID] {
			continue
		}
		prev, seen := announced[instruction.ServerID]
		action := "added"
		if seen {
			if prev.Hash == instruction.Hash {
				continue
			}
			action = "changed"
		}
		deltas = append(deltas, MCPInstructionDelta{
			ServerID: instruction.ServerID,
			Action:   action,
			Text:     instruction.Text,
			Hash:     instruction.Hash,
			Chars:    len(instruction.Text),
			Summary:  oneLineSummary(instruction.Text),
		})
	}
	for serverID := range announced {
		if disabled[serverID] {
			deltas = append(deltas, MCPInstructionDelta{ServerID: serverID, Action: "removed"})
			continue
		}
		if _, ok := current[serverID]; !ok {
			deltas = append(deltas, MCPInstructionDelta{ServerID: serverID, Action: "removed"})
		}
	}
	sort.Slice(deltas, func(i, j int) bool {
		if deltas[i].ServerID != deltas[j].ServerID {
			return deltas[i].ServerID < deltas[j].ServerID
		}
		return deltas[i].Action < deltas[j].Action
	})
	return deltas
}

func serverInstructionHash(serverID, text string) string {
	normalized := strings.Join(strings.Fields(text), " ")
	sum := sha256.Sum256([]byte(strings.TrimSpace(serverID) + "\n" + normalized))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func oneLineSummary(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 24 {
		return text[:24] + "..."
	}
	return text
}

func redactInstructionText(text string) string {
	parts := strings.Fields(text)
	for i, part := range parts {
		lower := strings.ToLower(part)
		switch {
		case strings.HasPrefix(lower, "password="),
			strings.HasPrefix(lower, "token="),
			strings.HasPrefix(lower, "secret="),
			strings.HasPrefix(part, "sk-"):
			parts[i] = "[REDACTED]"
		}
	}
	return strings.Join(parts, " ")
}
