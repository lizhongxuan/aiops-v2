package agentmgr

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type ResourceLockKey struct {
	ResourceType  string `json:"resourceType"`
	ResourceID    string `json:"resourceId"`
	OperationKind string `json:"operationKind"`
}

type ResourceLockLease struct {
	LeaseID   string          `json:"leaseId"`
	AgentID   string          `json:"agentId"`
	Key       ResourceLockKey `json:"key"`
	ExpiresAt time.Time       `json:"expiresAt"`
}

type ResourceLockDecision struct {
	Action string `json:"action"` // acquired | queued | denied
	Reason string `json:"reason,omitempty"`
	Holder string `json:"holder,omitempty"`
}

type ResourceLockResult struct {
	Acquired        bool            `json:"acquired"`
	Key             ResourceLockKey `json:"key"`
	OwnerAgentID    string          `json:"ownerAgentId,omitempty"`
	BlockingAgentID string          `json:"blockingAgentId,omitempty"`
	Reason          string          `json:"reason,omitempty"`
}

type ResourceLockManager struct {
	mu     sync.Mutex
	leases map[ResourceLockKey]map[string]ResourceLockLease
	ttl    time.Duration
}

func NewResourceLockManager() *ResourceLockManager {
	return &ResourceLockManager{
		leases: make(map[ResourceLockKey]map[string]ResourceLockLease),
		ttl:    10 * time.Minute,
	}
}

func (m *ResourceLockManager) TryAcquire(key ResourceLockKey, ownerAgentID string) (ResourceLockResult, error) {
	key = key.Normalize()
	if err := key.Validate(); err != nil {
		return ResourceLockResult{}, err
	}
	if strings.TrimSpace(ownerAgentID) == "" {
		return ResourceLockResult{}, fmt.Errorf("owner agent id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	m.purgeExpiredLocked(now)
	for held, owners := range m.leases {
		if _, ok := owners[ownerAgentID]; ok && held == key {
			return ResourceLockResult{Acquired: true, Key: key, OwnerAgentID: ownerAgentID}, nil
		}
		if resourceLocksConflict(held, key) {
			return ResourceLockResult{Acquired: false, Key: key, BlockingAgentID: firstLockOwner(owners), Reason: "resource_lock_conflict"}, nil
		}
	}
	if m.leases[key] == nil {
		m.leases[key] = map[string]ResourceLockLease{}
	}
	ttl := m.ttl
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	m.leases[key][ownerAgentID] = ResourceLockLease{
		LeaseID:   "lease:" + ownerAgentID + ":" + key.ResourceType + ":" + key.ResourceID + ":" + key.OperationKind,
		AgentID:   ownerAgentID,
		Key:       key,
		ExpiresAt: now.Add(ttl),
	}
	return ResourceLockResult{Acquired: true, Key: key, OwnerAgentID: ownerAgentID}, nil
}

func (m *ResourceLockManager) Release(key ResourceLockKey, ownerAgentID string) bool {
	key = key.Normalize()
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.leases[key][ownerAgentID]; !ok {
		return false
	}
	delete(m.leases[key], ownerAgentID)
	if len(m.leases[key]) == 0 {
		delete(m.leases, key)
	}
	return true
}

func (m *ResourceLockManager) purgeExpiredLocked(now time.Time) {
	for key, owners := range m.leases {
		for owner, lease := range owners {
			if !lease.ExpiresAt.IsZero() && !lease.ExpiresAt.After(now) {
				delete(owners, owner)
			}
		}
		if len(owners) == 0 {
			delete(m.leases, key)
		}
	}
}

func (k ResourceLockKey) Normalize() ResourceLockKey {
	k.ResourceType = strings.TrimSpace(strings.ToLower(k.ResourceType))
	k.ResourceID = strings.TrimSpace(k.ResourceID)
	k.OperationKind = strings.TrimSpace(strings.ToLower(k.OperationKind))
	return k
}

func (k ResourceLockKey) Validate() error {
	if k.ResourceType == "" {
		return fmt.Errorf("resource type is required")
	}
	if k.ResourceID == "" {
		return fmt.Errorf("resource id is required")
	}
	if k.OperationKind == "" {
		return fmt.Errorf("operation kind is required")
	}
	return nil
}

func resourceLocksConflict(a, b ResourceLockKey) bool {
	if a.ResourceType != b.ResourceType || a.ResourceID != b.ResourceID {
		return false
	}
	return isWriteOperation(a.OperationKind) || isWriteOperation(b.OperationKind)
}

func isWriteOperation(operation string) bool {
	switch strings.ToLower(strings.TrimSpace(operation)) {
	case "write", "mutate", "delete", "create", "update", "execute":
		return true
	default:
		return false
	}
}

func firstLockOwner(owners map[string]ResourceLockLease) string {
	for owner := range owners {
		return owner
	}
	return ""
}
