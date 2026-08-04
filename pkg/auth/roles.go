package auth

import (
	"fmt"
	"strings"
)

// Application roles embedded in JWTs and stored on users.role.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// ValidRole reports whether role is a known application role.
func ValidRole(role string) bool {
	switch role {
	case RoleUser, RoleAdmin:
		return true
	default:
		return false
	}
}

// EffectiveRole returns a usable role for token issuance. An empty role
// (for example rows created before the column existed) maps to RoleUser.
// Unknown non-empty values are rejected.
func EffectiveRole(role string) (string, error) {
	r := strings.TrimSpace(role)
	if r == "" {
		return RoleUser, nil
	}
	if !ValidRole(r) {
		return "", fmt.Errorf("invalid role %q", r)
	}
	return r, nil
}
