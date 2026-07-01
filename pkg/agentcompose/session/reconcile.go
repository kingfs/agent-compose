package session

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

const StalePendingSessionLastError = "session startup interrupted before runtime reached running state"

func ReconcilePendingState(ctx context.Context, store LifecycleStore, item *Session, startedAt time.Time) (*Session, error) {
	if item == nil || item.Summary.VMStatus != VMStatusPending {
		return item, nil
	}
	if !item.Summary.CreatedAt.Before(startedAt) {
		return item, nil
	}
	vmState, err := store.GetVMState(item.Summary.ID)
	if err != nil {
		return nil, err
	}
	if !vmState.StartedAt.IsZero() {
		return item, nil
	}
	now := time.Now().UTC()
	vmState.StoppedAt = now
	vmState.BoxID = ""
	if strings.TrimSpace(vmState.LastError) == "" {
		vmState.LastError = StalePendingSessionLastError
	}
	if err := store.SaveVMState(item.Summary.ID, vmState); err != nil {
		return nil, err
	}
	item.Summary.VMStatus = VMStatusFailed
	if err := store.UpdateSession(ctx, item); err != nil {
		return nil, err
	}
	_ = store.AddEvent(ctx, item.Summary.ID, SessionEvent{
		ID:        uuid.NewString(),
		Type:      "session.startup_interrupted",
		Level:     "warn",
		Message:   "session marked failed after a previous startup was interrupted before the VM became ready",
		CreatedAt: now,
	})
	return store.GetSession(ctx, item.Summary.ID)
}
