package workfloweditor

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
	"runner/server/service"
	contract "runner/workflow/edit"
)

var ErrWorkflowResourceUnavailable = mcp.ErrResourceUnavailable

const (
	workflowResourcePrefixV2        = "workflow://resource/v2/"
	workflowResourceSchemaVersionV2 = "aiops.workflow.resource.v2"
	workflowResourceTTL             = 15 * time.Minute
)

type WorkflowResourceContentV2 struct {
	URI         string          `json:"uri"`
	ContentType string          `json:"contentType"`
	Content     json.RawMessage `json:"content"`
	Digest      string          `json:"digest"`
	Version     string          `json:"version"`
}

type WorkflowContextResourceV2 struct {
	Key       contract.WorkflowKey
	ActorID   string
	SessionID string
	Revision  string
	Query     service.WorkflowContextQuery
}

type WorkflowCatalogResourceV2 struct {
	Key            contract.WorkflowKey
	ActorID        string
	SessionID      string
	CatalogVersion string
	Request        contract.ActionCatalogRequest
}

type workflowResourceDescriptorV2 struct {
	SchemaVersion string                    `json:"schemaVersion"`
	Kind          string                    `json:"kind"`
	ExpiresAt     int64                     `json:"expiresAt"`
	Context       WorkflowContextResourceV2 `json:"context,omitempty"`
	Catalog       WorkflowCatalogResourceV2 `json:"catalog,omitempty"`
}

// WorkflowResourcesV2 holds only an application reference and an AEAD key.
// Resource URIs contain encrypted, authenticated query descriptors, so no
// draft body or process-local descriptor cache is retained.
type WorkflowResourcesV2 struct {
	app       *service.WorkflowEditApplication
	key       [32]byte
	ephemeral bool
	now       func() time.Time
	ttl       time.Duration
}

// NewWorkflowResourcesV2 creates an explicitly ephemeral signer for tests and
// in-process use. Production registration rejects it; use the stable-key
// constructor so issued URIs remain readable after a restart.
func NewWorkflowResourcesV2(app *service.WorkflowEditApplication) (*WorkflowResourcesV2, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("workflow resources: initialize ephemeral signer: %w", err)
	}
	return newWorkflowResourcesV2(app, key, true)
}

func NewWorkflowResourcesV2WithKey(app *service.WorkflowEditApplication, key []byte) (*WorkflowResourcesV2, error) {
	return newWorkflowResourcesV2(app, key, false)
}

func newWorkflowResourcesV2(app *service.WorkflowEditApplication, key []byte, ephemeral bool) (*WorkflowResourcesV2, error) {
	if app == nil {
		return nil, fmt.Errorf("workflow resources: application is required")
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("workflow resources: signer key must be exactly 32 bytes")
	}
	provider := &WorkflowResourcesV2{app: app, ephemeral: ephemeral, now: time.Now, ttl: workflowResourceTTL}
	copy(provider.key[:], key)
	return provider, nil
}

func (p *WorkflowResourcesV2) BindContext(value WorkflowContextResourceV2) (tooling.ResultReference, error) {
	if value.Key.Validate() != nil || !validWorkflowResourceIdentity(value.ActorID, value.SessionID) || strings.TrimSpace(value.Revision) == "" {
		return tooling.ResultReference{}, ErrWorkflowResourceUnavailable
	}
	value.Query.Selection.NodeIDs = append([]string(nil), value.Query.Selection.NodeIDs...)
	value.Query.Selection.EdgeIDs = append([]string(nil), value.Query.Selection.EdgeIDs...)
	return p.bind(workflowResourceDescriptorV2{Kind: "context", Context: value}, value.Revision, "Workflow context")
}

func (p *WorkflowResourcesV2) BindCatalog(value WorkflowCatalogResourceV2) (tooling.ResultReference, error) {
	if value.Key.Validate() != nil || !validWorkflowResourceIdentity(value.ActorID, value.SessionID) || strings.TrimSpace(value.CatalogVersion) == "" || value.Request.WorkflowKey != value.Key {
		return tooling.ResultReference{}, ErrWorkflowResourceUnavailable
	}
	value.Request.Names = append([]string(nil), value.Request.Names...)
	return p.bind(workflowResourceDescriptorV2{Kind: "catalog", Catalog: value}, value.CatalogVersion, "Workflow action catalog")
}

func (p *WorkflowResourcesV2) bind(item workflowResourceDescriptorV2, version, title string) (tooling.ResultReference, error) {
	if p == nil || p.app == nil || p.now == nil || p.ttl <= 0 {
		return tooling.ResultReference{}, ErrWorkflowResourceUnavailable
	}
	item.SchemaVersion = workflowResourceSchemaVersionV2
	item.ExpiresAt = p.now().Add(p.ttl).Unix()
	token, err := p.seal(item)
	if err != nil || len(token) > 64<<10 {
		return tooling.ResultReference{}, ErrWorkflowResourceUnavailable
	}
	return tooling.ResultReference{Kind: tooling.ResultReferenceKindMCPResource, URI: workflowResourcePrefixV2 + token, Title: title, Summary: "按需读取当前授权范围内的不可变版本", ContentType: "application/json", Version: version}, nil
}

func (p *WorkflowResourcesV2) Read(ctx context.Context, uri string) (WorkflowResourceContentV2, error) {
	item, err := p.openURI(uri)
	if err != nil {
		return WorkflowResourceContentV2{}, ErrWorkflowResourceUnavailable
	}
	var value any
	var version string
	switch item.Kind {
	case "context":
		if !resourceScopeMatches(ctx, item.Context.Key, item.Context.ActorID, item.Context.SessionID) {
			return WorkflowResourceContentV2{}, ErrWorkflowResourceUnavailable
		}
		result, readErr := p.app.GetContext(ctx, item.Context.Key, item.Context.Query)
		if readErr != nil || result.Revision.ResourceVersion != item.Context.Revision {
			return WorkflowResourceContentV2{}, ErrWorkflowResourceUnavailable
		}
		value, version = workflowContextContract(result, uri, true), result.Revision.ResourceVersion
	case "catalog":
		if !resourceScopeMatches(ctx, item.Catalog.Key, item.Catalog.ActorID, item.Catalog.SessionID) {
			return WorkflowResourceContentV2{}, ErrWorkflowResourceUnavailable
		}
		result, readErr := readWorkflowActionCatalog(ctx, p.app, item.Catalog.Request)
		if readErr != nil || result.CatalogVersion != item.Catalog.CatalogVersion {
			return WorkflowResourceContentV2{}, ErrWorkflowResourceUnavailable
		}
		value, version = result, result.CatalogVersion
	default:
		return WorkflowResourceContentV2{}, ErrWorkflowResourceUnavailable
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return WorkflowResourceContentV2{}, ErrWorkflowResourceUnavailable
	}
	sum := sha256.Sum256(raw)
	return WorkflowResourceContentV2{URI: uri, ContentType: "application/json", Content: raw, Digest: fmt.Sprintf("sha256:%x", sum), Version: version}, nil
}

func (p *WorkflowResourcesV2) seal(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	aead, err := p.aead()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, raw, []byte(workflowResourcePrefixV2))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (p *WorkflowResourcesV2) openURI(uri string) (workflowResourceDescriptorV2, error) {
	var item workflowResourceDescriptorV2
	if p == nil || p.app == nil || p.now == nil || !strings.HasPrefix(uri, workflowResourcePrefixV2) {
		return item, ErrWorkflowResourceUnavailable
	}
	token := strings.TrimPrefix(uri, workflowResourcePrefixV2)
	if strings.Contains(token, "/") || p.openToken(token, &item) != nil || item.SchemaVersion != workflowResourceSchemaVersionV2 || item.ExpiresAt <= p.now().Unix() || !item.valid() {
		return item, ErrWorkflowResourceUnavailable
	}
	return item, nil
}

func (p *WorkflowResourcesV2) openToken(token string, target any) error {
	if p == nil || p.app == nil || p.now == nil || target == nil || token == "" || len(token) > 64<<10 {
		return ErrWorkflowResourceUnavailable
	}
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return ErrWorkflowResourceUnavailable
	}
	aead, err := p.aead()
	if err != nil || len(sealed) < aead.NonceSize() {
		return ErrWorkflowResourceUnavailable
	}
	nonce, ciphertext := sealed[:aead.NonceSize()], sealed[aead.NonceSize():]
	raw, err := aead.Open(nil, nonce, ciphertext, []byte(workflowResourcePrefixV2))
	if err != nil || contract.DecodeStrict(raw, target) != nil {
		return ErrWorkflowResourceUnavailable
	}
	return nil
}

func (p *WorkflowResourcesV2) aead() (cipher.AEAD, error) {
	block, err := aes.NewCipher(p.key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (d workflowResourceDescriptorV2) valid() bool {
	switch d.Kind {
	case "context":
		return d.Context.Key.Validate() == nil && validWorkflowResourceIdentity(d.Context.ActorID, d.Context.SessionID) && strings.TrimSpace(d.Context.Revision) != ""
	case "catalog":
		return d.Catalog.Key.Validate() == nil && validWorkflowResourceIdentity(d.Catalog.ActorID, d.Catalog.SessionID) && strings.TrimSpace(d.Catalog.CatalogVersion) != "" && d.Catalog.Request.WorkflowKey == d.Catalog.Key
	default:
		return false
	}
}

func validWorkflowResourceIdentity(actorID, sessionID string) bool {
	return actorID != "" && actorID == strings.TrimSpace(actorID) && sessionID != "" && sessionID == strings.TrimSpace(sessionID)
}

func resourceScopeMatches(ctx context.Context, key contract.WorkflowKey, actorID, sessionID string) bool {
	scope, err := trustedWorkflowScope(ctx, key)
	return err == nil && scope.UserID == actorID && scope.SessionID == sessionID
}
