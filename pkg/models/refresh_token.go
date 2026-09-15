package models

import "time"

// RefreshToken is a persisted opaque refresh credential (hash only).
// FamilyID groups rotated tokens so reuse of a consumed token can revoke the chain.
type RefreshToken struct {
	ID         uint       `json:"id"`
	UserID     uint       `json:"user_id"`
	TokenHash  string     `json:"-"`
	FamilyID   string     `json:"-"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
