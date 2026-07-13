package eval

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"aiops-v2/internal/modeltrace"
	"aiops-v2/internal/runtimekernel"
)

type ReplayBaseline struct {
	EventCount    int                   `json:"eventCount"`
	Events        []ReplayBaselineEvent `json:"events"`
	RolloutHash   string                `json:"rolloutHash"`
	TransportHash string                `json:"transportHash"`
}

type ReplayBaselineEvent struct {
	Sequence int64  `json:"sequence"`
	Kind     string `json:"kind"`
	Hash     string `json:"hash"`
}

func replaySourceContentHash(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func replayExecutionBaseline(execution ReplayExecution) (ReplayBaseline, error) {
	eventHashes, err := normalizedReplayEventHashes(execution.Rollout, execution.Contract)
	if err != nil {
		return ReplayBaseline{}, err
	}
	rolloutHash, err := normalizedReplayFactHash(eventHashes)
	if err != nil {
		return ReplayBaseline{}, err
	}
	transportHash := ""
	if execution.TransportState != nil {
		transportHash, err = normalizedReplayFactHash(*execution.TransportState)
		if err != nil {
			return ReplayBaseline{}, err
		}
	}
	events := make([]ReplayBaselineEvent, len(execution.Rollout))
	for index, event := range execution.Rollout {
		events[index] = ReplayBaselineEvent{Sequence: event.Sequence, Kind: event.Kind, Hash: eventHashes[index]}
	}
	return ReplayBaseline{EventCount: len(events), Events: events, RolloutHash: rolloutHash, TransportHash: transportHash}, nil
}

func compareReplayBaselines(expected, actual ReplayBaseline) error {
	limit := len(expected.Events)
	if len(actual.Events) < limit {
		limit = len(actual.Events)
	}
	for index := 0; index < limit; index++ {
		want, got := expected.Events[index], actual.Events[index]
		if want.Sequence != got.Sequence || want.Kind != got.Kind || want.Hash != got.Hash {
			kind := want.Kind
			sequence := want.Sequence
			if kind == "" {
				kind, sequence = got.Kind, got.Sequence
			}
			return &ReplayDivergenceError{Sequence: sequence, ExpectedKind: want.Kind, ActualKind: got.Kind, ExpectedHash: want.Hash, ActualHash: got.Hash, OwnerModule: replayOwner(kind)}
		}
	}
	if len(expected.Events) != len(actual.Events) {
		if len(expected.Events) > limit {
			want := expected.Events[limit]
			return &ReplayDivergenceError{Sequence: want.Sequence, ExpectedKind: want.Kind, ExpectedHash: want.Hash, OwnerModule: replayOwner(want.Kind)}
		}
		got := actual.Events[limit]
		return &ReplayDivergenceError{Sequence: got.Sequence, ActualKind: got.Kind, ActualHash: got.Hash, OwnerModule: replayOwner(got.Kind)}
	}
	if expected.RolloutHash != actual.RolloutHash {
		return errors.New("rollout replay baseline aggregate hash does not match its event hashes")
	}
	if expected.TransportHash != actual.TransportHash {
		sequence := int64(0)
		for _, event := range expected.Events {
			if event.Kind == modeltrace.CanonicalRolloutKindTransportProjection {
				sequence = event.Sequence
				break
			}
		}
		return &ReplayDivergenceError{Sequence: sequence, ExpectedKind: modeltrace.CanonicalRolloutKindTransportProjection, ActualKind: modeltrace.CanonicalRolloutKindTransportProjection, ExpectedHash: expected.TransportHash, ActualHash: actual.TransportHash, OwnerModule: "appui.TransportProjector"}
	}
	return nil
}

func validateReplayBaseline(baseline ReplayBaseline) error {
	if baseline.EventCount <= 0 || baseline.EventCount != len(baseline.Events) || !strings.HasPrefix(baseline.RolloutHash, "sha256:") || !strings.HasPrefix(baseline.TransportHash, "sha256:") {
		return errors.New("rollout replay reference fixture requires a complete baseline")
	}
	hashes := make([]string, len(baseline.Events))
	for index, event := range baseline.Events {
		if event.Sequence != int64(index+1) || strings.TrimSpace(event.Kind) == "" || !strings.HasPrefix(event.Hash, "sha256:") {
			return fmt.Errorf("rollout replay baseline event[%d] is invalid", index)
		}
		hashes[index] = event.Hash
	}
	aggregate, err := normalizedReplayFactHash(hashes)
	if err != nil {
		return err
	}
	if aggregate != baseline.RolloutHash {
		return errors.New("rollout replay baseline aggregate hash does not match its event hashes")
	}
	return nil
}

func normalizedReplayEventHashes(events []modeltrace.CanonicalRolloutEvent, contracts ...ReplayContractArtifacts) ([]string, error) {
	var contract ReplayContractArtifacts
	if len(contracts) > 0 {
		contract = contracts[0]
	}
	tokenHashes, err := normalizedReplayActionTokenHashes(contract.ActionTokens)
	if err != nil {
		return nil, err
	}
	normalizer := replayFactNormalizer{ids: make(map[string]map[string]string)}
	hashes := make([]string, len(events))
	for index, event := range events {
		if event.Kind == modeltrace.CanonicalRolloutKindApprovalRequested {
			data, cloneErr := json.Marshal(event.Payload)
			if cloneErr != nil {
				return nil, cloneErr
			}
			var payload map[string]any
			if cloneErr := json.Unmarshal(data, &payload); cloneErr != nil {
				return nil, cloneErr
			}
			if raw, _ := payload["actionTokenHash"].(string); tokenHashes[raw] != "" {
				payload["actionTokenHash"] = tokenHashes[raw]
				event.Payload = payload
			}
		}
		event.Hash = ""
		hash, err := normalizer.hash(event)
		if err != nil {
			return nil, err
		}
		hashes[index] = hash
	}
	return hashes, nil
}

func normalizedReplayActionTokenHashes(tokens []runtimekernel.ActionToken) (map[string]string, error) {
	hashes := make(map[string]string, len(tokens))
	for _, token := range tokens {
		if err := token.Validate(); err != nil {
			return nil, fmt.Errorf("validate replay action token: %w", err)
		}
		raw := token.Hash
		token.ExpiresAt = time.Unix(0, 0).UTC().Add(24 * time.Hour)
		normalized, err := runtimekernel.FreezeActionToken(token)
		if err != nil {
			return nil, fmt.Errorf("normalize replay action token clock: %w", err)
		}
		hashes[raw] = normalized.Hash
	}
	return hashes, nil
}

func normalizedReplayFactHash(value any) (string, error) {
	return (&replayFactNormalizer{ids: make(map[string]map[string]string)}).hash(value)
}

type replayFactNormalizer struct {
	ids map[string]map[string]string
}

func (n *replayFactNormalizer) hash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal replay fact: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return "", fmt.Errorf("decode replay fact: %w", err)
	}
	normalized := n.normalize("", decoded)
	normalizedJSON, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal normalized replay fact: %w", err)
	}
	digest := sha256.Sum256(normalizedJSON)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func (n *replayFactNormalizer) normalize(key string, value any) any {
	if replayTimeKey(key) && value != nil && fmt.Sprint(value) != "" {
		return "<wall-clock>"
	}
	if replayRandomIDKey(key) {
		if text, ok := value.(string); ok && text != "" {
			values := n.ids[key]
			if values == nil {
				values = make(map[string]string)
				n.ids[key] = values
			}
			if values[text] == "" {
				values[text] = fmt.Sprintf("<%s:%d>", key, len(values)+1)
			}
			return values[text]
		}
	}
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for childKey := range typed {
			keys = append(keys, childKey)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(typed))
		for _, childKey := range keys {
			out[childKey] = n.normalize(childKey, typed[childKey])
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index := range typed {
			out[index] = n.normalize(key, typed[index])
		}
		return out
	default:
		return typed
	}
}

func replayTimeKey(key string) bool {
	switch normalizeReplayKey(key) {
	case "createdat", "updatedat", "startedat", "finishedat", "completedat", "requestedat", "resolvedat", "timestamp", "durationms", "elapsedms", "latencyms":
		return true
	default:
		return false
	}
}

func replayRandomIDKey(key string) bool {
	switch normalizeReplayKey(key) {
	case "messageid", "spanid", "traceid":
		return true
	default:
		return false
	}
}

func normalizeReplayKey(key string) string {
	return strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "").Replace(strings.TrimSpace(key)))
}
