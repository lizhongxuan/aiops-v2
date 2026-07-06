package specialinputmemory

import (
	"time"
)

type GCInput struct {
	Now    time.Time
	TurnID string
}

func ApplyGC(state SessionSpecialInputState, input GCInput) (SessionSpecialInputState, []SpecialInputMemoryEvent) {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	next := state.Clone()
	var events []SpecialInputMemoryEvent
	for i := range next.Facts {
		if next.Facts[i].Status == FactStatusActive && !next.Facts[i].ExpiresAt.IsZero() && now.After(next.Facts[i].ExpiresAt) {
			next.Facts[i].Status = FactStatusExpired
			events = appendEvent(events, SpecialInputMemoryEvent{
				Type:         "fact_expired",
				FactID:       next.Facts[i].ID,
				CanonicalKey: next.Facts[i].CanonicalKey,
			}, input.TurnID, now)
		}
		if next.Facts[i].Status == FactStatusActive {
			next.Facts[i].Weight -= 0.15
			if next.Facts[i].Weight < 0 {
				next.Facts[i].Weight = 0
			}
		}
	}
	for i := range next.Grants {
		if next.Grants[i].Status == GrantStatusActive && next.Grants[i].Expired(now) {
			next.Grants[i].Status = GrantStatusExpired
			next.Grants[i].RevokedReason = "ttl_expired"
			events = appendEvent(events, SpecialInputMemoryEvent{
				Type:         "grant_expired",
				GrantID:      next.Grants[i].ID,
				CanonicalKey: next.Grants[i].CanonicalKey,
				Reason:       "ttl_expired",
			}, input.TurnID, now)
		}
		if next.Grants[i].Status == GrantStatusActive {
			next.Grants[i].Weight -= 0.15
			if next.Grants[i].Weight < 0 {
				next.Grants[i].Weight = 0
			}
		}
	}
	tombstones := next.Tombstones[:0]
	for _, tombstone := range next.Tombstones {
		if !tombstone.ExpiresAt.IsZero() && now.After(tombstone.ExpiresAt) {
			events = appendEvent(events, SpecialInputMemoryEvent{
				Type:         "tombstone_expired",
				CanonicalKey: tombstone.CanonicalKey,
				Reason:       tombstone.Reason,
			}, input.TurnID, now)
			continue
		}
		tombstones = append(tombstones, tombstone)
	}
	next.Tombstones = tombstones
	next.UpdatedAt = now
	return next, events
}
