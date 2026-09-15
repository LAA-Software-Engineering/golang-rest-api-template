package repository

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToInt64(t *testing.T) {
	assert.Equal(t, int64(0), toInt64(0))
	assert.Equal(t, int64(42), toInt64(42))
	assert.Equal(t, int64(math.MaxInt64), toInt64(uint(math.MaxInt64)))
	// Values above math.MaxInt64 clamp instead of wrapping negative.
	assert.Equal(t, int64(math.MaxInt64), toInt64(math.MaxUint64))
}
