package models

import "time"

// RefreshToken is a persisted opaque refresh credential (hash only).
// FamilyID groups rotated tokens so reuse of a consumed token can revoke the chain.
type RefreshToken struct {
	ID         uint       `json:"id" gorm:"primaryKey"`
	UserID     uint       `json:"user_id" gorm:"index;not null"`
	TokenHash  string     `json:"-" gorm:"uniqueIndex;size:64;not null"`
	FamilyID   string     `json:"-" gorm:"index;size:64;not null"`
	ExpiresAt  time.Time  `json:"expires_at" gorm:"index;not null"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt  time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}
