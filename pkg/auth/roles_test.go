package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidRole(t *testing.T) {
	assert.True(t, ValidRole(RoleUser))
	assert.True(t, ValidRole(RoleAdmin))
	assert.False(t, ValidRole(""))
	assert.False(t, ValidRole("superuser"))
	assert.False(t, ValidRole("USER"))
}

func TestEffectiveRole(t *testing.T) {
	got, err := EffectiveRole("")
	assert.NoError(t, err)
	assert.Equal(t, RoleUser, got)

	got, err = EffectiveRole("  admin  ")
	assert.NoError(t, err)
	assert.Equal(t, RoleAdmin, got)

	_, err = EffectiveRole("root")
	assert.Error(t, err)
}
