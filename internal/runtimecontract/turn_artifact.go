package runtimecontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const TurnArtifactSchemaVersion = "aiops.turn.artifact.v1"

type TurnArtifactEnvelope struct {
	SchemaVersion string                   `json:"schemaVersion"`
	ID            string                   `json:"id"`
	Kind          string                   `json:"kind"`
	Digest        string                   `json:"digest"`
	ContentType   string                   `json:"contentType"`
	Inline        json.RawMessage          `json:"inline,omitempty"`
	Resource      *TurnArtifactResourceRef `json:"resource,omitempty"`
}

type TurnArtifactResourceRef struct {
	URI         string `json:"uri"`
	Digest      string `json:"digest"`
	ContentType string `json:"contentType"`
	Bytes       int64  `json:"bytes,omitempty"`
}

func NewInlineTurnArtifact(kind string, value any) (TurnArtifactEnvelope, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return TurnArtifactEnvelope{}, err
	}
	compact, err := compactTurnArtifactJSON(raw)
	if err != nil {
		return TurnArtifactEnvelope{}, err
	}
	digest := turnArtifactDigest(compact)
	artifact := TurnArtifactEnvelope{SchemaVersion: TurnArtifactSchemaVersion, ID: turnArtifactID(kind, digest), Kind: strings.TrimSpace(kind), Digest: digest, ContentType: "application/json", Inline: compact}
	return artifact, artifact.Validate()
}

func NewResourceTurnArtifact(kind string, resource TurnArtifactResourceRef) (TurnArtifactEnvelope, error) {
	artifact := TurnArtifactEnvelope{SchemaVersion: TurnArtifactSchemaVersion, ID: turnArtifactID(kind, resource.Digest), Kind: strings.TrimSpace(kind), Digest: resource.Digest, ContentType: resource.ContentType, Resource: &resource}
	return artifact, artifact.Validate()
}

func (artifact TurnArtifactEnvelope) Validate() error {
	if artifact.SchemaVersion != TurnArtifactSchemaVersion || strings.TrimSpace(artifact.Kind) == "" || strings.TrimSpace(artifact.ContentType) == "" || !validTurnArtifactDigest(artifact.Digest) || artifact.ID != turnArtifactID(artifact.Kind, artifact.Digest) {
		return fmt.Errorf("invalid turn artifact identity")
	}
	hasInline, hasResource := len(artifact.Inline) > 0, artifact.Resource != nil
	if hasInline == hasResource {
		return fmt.Errorf("turn artifact requires exactly one inline body or resource reference")
	}
	if hasInline {
		compact, err := compactTurnArtifactJSON(artifact.Inline)
		if err != nil || artifact.ContentType != "application/json" || artifact.Digest != turnArtifactDigest(compact) {
			return fmt.Errorf("turn artifact inline digest mismatch")
		}
		return nil
	}
	resource := artifact.Resource
	if strings.TrimSpace(resource.URI) == "" || resource.Digest != artifact.Digest || resource.ContentType != artifact.ContentType || resource.Bytes < 0 || !validTurnArtifactDigest(resource.Digest) {
		return fmt.Errorf("invalid turn artifact resource reference")
	}
	return nil
}

func (artifact TurnArtifactEnvelope) Clone() TurnArtifactEnvelope {
	cloned := artifact
	cloned.Inline = append(json.RawMessage(nil), artifact.Inline...)
	if artifact.Resource != nil {
		resource := *artifact.Resource
		cloned.Resource = &resource
	}
	return cloned
}

func compactTurnArtifactJSON(raw []byte) (json.RawMessage, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, fmt.Errorf("invalid turn artifact JSON: %w", err)
	}
	return append(json.RawMessage(nil), compact.Bytes()...), nil
}

func turnArtifactDigest(content []byte) string {
	digest := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func validTurnArtifactDigest(value string) bool {
	encoded := strings.TrimPrefix(value, "sha256:")
	if encoded == value || len(encoded) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(encoded)
	return err == nil
}

func turnArtifactID(kind, digest string) string {
	kind = strings.TrimSpace(kind)
	encoded := strings.TrimPrefix(digest, "sha256:")
	if kind == "" || len(encoded) < 16 {
		return ""
	}
	return "artifact:" + kind + ":" + encoded[:16]
}
