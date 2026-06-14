package actionproposal

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Signer struct {
	secret []byte
	now    func() time.Time
}

func NewSigner(secret []byte, now func() time.Time) *Signer {
	if now == nil {
		now = time.Now
	}
	return &Signer{secret: append([]byte(nil), secret...), now: now}
}

func (s *Signer) Sign(claims ActionTokenClaims) (string, error) {
	if len(s.secret) == 0 {
		return "", errors.New("action token secret is required")
	}
	if claims.ExpiresAt.IsZero() {
		return "", errors.New("expiresAt is required")
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	sig := s.sign([]byte(encodedPayload))
	return encodedPayload + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (s *Signer) Parse(token string) (ActionTokenClaims, error) {
	if len(s.secret) == 0 {
		return ActionTokenClaims{}, errors.New("action token secret is required")
	}
	payload, sig, err := splitToken(token)
	if err != nil {
		return ActionTokenClaims{}, err
	}
	if !hmac.Equal(sig, s.sign([]byte(payload))) {
		return ActionTokenClaims{}, errors.New("invalid action token signature")
	}
	data, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return ActionTokenClaims{}, errors.New("invalid action token payload")
	}
	var claims ActionTokenClaims
	if err := json.Unmarshal(data, &claims); err != nil {
		return ActionTokenClaims{}, err
	}
	return claims, nil
}

func (s *Signer) Verify(token string, expected ActionTokenClaims) (ActionTokenClaims, error) {
	claims, err := s.Parse(token)
	if err != nil {
		return ActionTokenClaims{}, err
	}
	if !claims.ExpiresAt.After(s.now()) {
		return ActionTokenClaims{}, errors.New("action token expired")
	}
	if err := claims.Match(expected); err != nil {
		return ActionTokenClaims{}, err
	}
	return claims, nil
}

func (c ActionTokenClaims) Match(expected ActionTokenClaims) error {
	checks := []struct {
		name string
		got  string
		want string
	}{
		{name: "sessionId", got: c.SessionID, want: expected.SessionID},
		{name: "turnId", got: c.TurnID, want: expected.TurnID},
		{name: "tenantId", got: c.TenantID, want: expected.TenantID},
		{name: "userId", got: c.UserID, want: expected.UserID},
		{name: "incidentId", got: c.IncidentID, want: expected.IncidentID},
		{name: "toolName", got: c.ToolName, want: expected.ToolName},
		{name: "inputHash", got: c.InputHash, want: expected.InputHash},
		{name: "source", got: string(c.Source), want: string(expected.Source)},
		{name: "risk", got: string(c.Risk), want: string(expected.Risk)},
	}
	for _, check := range checks {
		if strings.TrimSpace(check.want) == "" {
			continue
		}
		if check.got != check.want {
			return fmt.Errorf("action token %s mismatch", check.name)
		}
	}
	return nil
}

func (s *Signer) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}

func splitToken(token string) (string, []byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", nil, errors.New("invalid action token format")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", nil, errors.New("invalid action token signature")
	}
	return parts[0], sig, nil
}

func NormalizedInputHash(input json.RawMessage) (string, error) {
	normalized, err := NormalizeInput(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(normalized)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func NormalizeInput(input json.RawMessage) ([]byte, error) {
	if len(bytes.TrimSpace(input)) == 0 {
		input = json.RawMessage(`{}`)
	}
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	value = normalizeValue(value)
	if obj, ok := value.(map[string]any); ok {
		value = normalizeCommandObject(obj)
	}
	return json.Marshal(value)
}

func normalizeValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			if key == "actionToken" || key == "intent" {
				continue
			}
			out[key] = normalizeValue(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = normalizeValue(item)
		}
		return out
	default:
		return v
	}
}

func normalizeCommandObject(obj map[string]any) map[string]any {
	command := stringValue(obj["command"])
	args := stringArray(obj["args"])
	if command == "" {
		fields := strings.Fields(stringValue(obj["cmd"]))
		if len(fields) > 0 {
			command = fields[0]
			args = fields[1:]
		}
	}
	if command == "" {
		return obj
	}
	out := make(map[string]any, len(obj)+1)
	for key, value := range obj {
		if key == "cmd" || key == "command" || key == "args" || key == "actionToken" || key == "intent" {
			continue
		}
		out[key] = value
	}
	out["command"] = command
	out["args"] = args
	return out
}

func stringValue(value any) string {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func stringArray(value any) []string {
	switch v := value.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := stringValue(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string(nil), v...)
	default:
		return nil
	}
}
