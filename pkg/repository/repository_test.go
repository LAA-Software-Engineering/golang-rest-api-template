package repository

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestIsBookNotFound(t *testing.T) {
	assert.True(t, IsBookNotFound(gorm.ErrRecordNotFound))
	assert.False(t, IsBookNotFound(nil))
	assert.False(t, IsBookNotFound(errors.New("other")))
	assert.True(t, IsBookNotFound(errors.Join(gorm.ErrRecordNotFound, errors.New("wrap"))))
}

func TestIsUserNotFound(t *testing.T) {
	assert.True(t, IsUserNotFound(gorm.ErrRecordNotFound))
	assert.False(t, IsUserNotFound(nil))
	assert.False(t, IsUserNotFound(errors.New("other")))
	assert.True(t, IsUserNotFound(errors.Join(gorm.ErrRecordNotFound, errors.New("wrap"))))
}
