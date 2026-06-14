package agentmgr

import (
	"testing"
	"time"
)

func TestResourceLockManagerRejectsConflictingAcquire(t *testing.T) {
	manager := NewResourceLockManager()
	key := ResourceLockKey{ResourceType: "service", ResourceID: "synthetic.service-a", OperationKind: "write"}
	first, err := manager.TryAcquire(key, "agent-1")
	if err != nil || !first.Acquired {
		t.Fatalf("first acquire = %#v err=%v, want acquired", first, err)
	}
	second, err := manager.TryAcquire(key, "agent-2")
	if err != nil {
		t.Fatalf("second acquire err = %v", err)
	}
	if second.Acquired || second.BlockingAgentID != "agent-1" {
		t.Fatalf("second acquire = %#v, want blocked by agent-1", second)
	}
}

func TestResourceLockReadAndWriteConflictByResource(t *testing.T) {
	manager := NewResourceLockManager()
	read := ResourceLockKey{ResourceType: "service", ResourceID: "synthetic.service-a", OperationKind: "read"}
	write := ResourceLockKey{ResourceType: "service", ResourceID: "synthetic.service-a", OperationKind: "write"}
	if result, err := manager.TryAcquire(read, "reader"); err != nil || !result.Acquired {
		t.Fatalf("read acquire = %#v err=%v, want acquired", result, err)
	}
	if result, err := manager.TryAcquire(write, "writer"); err != nil || result.Acquired {
		t.Fatalf("write acquire = %#v err=%v, want blocked", result, err)
	}
}

func TestResourceLockAllowsParallelReads(t *testing.T) {
	manager := NewResourceLockManager()
	key := ResourceLockKey{ResourceType: "service", ResourceID: "synthetic.service-a", OperationKind: "read"}
	if result, err := manager.TryAcquire(key, "reader-1"); err != nil || !result.Acquired {
		t.Fatalf("first read acquire = %#v err=%v, want acquired", result, err)
	}
	if result, err := manager.TryAcquire(key, "reader-2"); err != nil || !result.Acquired {
		t.Fatalf("second read acquire = %#v err=%v, want acquired", result, err)
	}
}

func TestResourceLockManagerPurgesExpiredLeases(t *testing.T) {
	manager := NewResourceLockManager()
	manager.ttl = time.Nanosecond
	key := ResourceLockKey{ResourceType: "file", ResourceID: "config://service-a", OperationKind: "write"}
	if result, err := manager.TryAcquire(key, "worker-a"); err != nil || !result.Acquired {
		t.Fatalf("first acquire = %#v err=%v, want acquired", result, err)
	}
	time.Sleep(time.Millisecond)
	if result, err := manager.TryAcquire(key, "worker-b"); err != nil || !result.Acquired {
		t.Fatalf("second acquire = %#v err=%v, want acquired after ttl purge", result, err)
	}
}
