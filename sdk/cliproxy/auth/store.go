package auth

import (
	"context"
	"time"
)

// Store abstracts persistence of Auth state across restarts.
type Store interface {
	// List returns all auth records stored in the backend.
	List(ctx context.Context) ([]*Auth, error)
	// Save persists the provided auth record, replacing any existing one with same ID.
	Save(ctx context.Context, auth *Auth) (string, error)
	// Delete removes the auth record identified by id.
	Delete(ctx context.Context, id string) error
}

// RuntimeFreezeState persists request-blocking cooldowns outside credential JSON.
type RuntimeFreezeState struct {
	AuthID        string    `json:"auth_id"`
	Reason        string    `json:"reason"`
	NextRecoverAt time.Time `json:"next_recover_at"`
}

// RuntimeFreezeStore is an optional sidecar store for runtime cooldown state.
type RuntimeFreezeStore interface {
	ListRuntimeFreezes(ctx context.Context) ([]RuntimeFreezeState, error)
	SaveRuntimeFreeze(ctx context.Context, state RuntimeFreezeState) error
	DeleteRuntimeFreeze(ctx context.Context, authID string) error
}
