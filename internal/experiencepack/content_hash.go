package experiencepack

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

func CanonicalJSON(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		return nil, err
	}
	generic = stripAssetID(generic)
	return json.Marshal(generic)
}

func HashCanonicalJSON(value any) (AssetID, error) {
	canonical, err := CanonicalJSON(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return AssetID("sha256:" + hex.EncodeToString(sum[:])), nil
}

func NormalizeMarkdown(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n")) + "\n"
}

func HashSkillMarkdown(content string) AssetID {
	sum := sha256.Sum256([]byte(NormalizeMarkdown(content)))
	return AssetID("sha256:" + hex.EncodeToString(sum[:]))
}

func VerifyAssetID(value any, assetID AssetID) error {
	next, err := HashCanonicalJSON(value)
	if err != nil {
		return err
	}
	if next != assetID {
		return fmt.Errorf("%w: expected %s got %s", ErrValidationFailed, assetID, next)
	}
	return nil
}

func VerifyStoredAssetID(value any, assetID AssetID) error {
	if !ValidAssetID(assetID) {
		return fmt.Errorf("%w: invalid asset_id %q", ErrValidationFailed, assetID)
	}
	switch typed := value.(type) {
	case SkillAsset:
		next := HashSkillMarkdown(typed.Content)
		if next != assetID {
			return fmt.Errorf("%w: expected %s got %s", ErrValidationFailed, assetID, next)
		}
	default:
		if err := VerifyAssetID(value, assetID); err != nil {
			return err
		}
	}
	return nil
}

func MustHashCanonicalJSON(value any) AssetID {
	id, err := HashCanonicalJSON(value)
	if err != nil {
		panic(err)
	}
	return id
}

func stripAssetID(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		next := make(map[string]any, len(typed))
		for key, entry := range typed {
			if key == "asset_id" || key == "assetId" {
				continue
			}
			next[key] = stripAssetID(entry)
		}
		return next
	case []any:
		next := make([]any, len(typed))
		for i, entry := range typed {
			next[i] = stripAssetID(entry)
		}
		return next
	default:
		return typed
	}
}

func canonicalEqual(left, right any) bool {
	leftJSON, leftErr := CanonicalJSON(left)
	rightJSON, rightErr := CanonicalJSON(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return bytes.Equal(leftJSON, rightJSON)
}
