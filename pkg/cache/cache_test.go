package cache

import (
	"context"
	"testing"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestNewRedisClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	redisClient := NewMockCache(ctrl)
	assert.NotNil(t, redisClient)
}

func TestMockCache_GetExpectation(t *testing.T) {
	ctrl := gomock.NewController(t)
	mock := NewMockCache(ctrl)
	ctx := context.Background()
	const key = "test:key"
	want := `["a"]`
	mock.EXPECT().Get(gomock.Any(), key).Return(redis.NewStringResult(want, nil))

	got, err := mock.Get(ctx, key).Result()
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestMockCache_GetExpectation_miss(t *testing.T) {
	ctrl := gomock.NewController(t)
	mock := NewMockCache(ctrl)
	ctx := context.Background()
	mock.EXPECT().Get(gomock.Any(), "missing").Return(redis.NewStringResult("", redis.Nil))

	_, err := mock.Get(ctx, "missing").Result()
	assert.ErrorIs(t, err, redis.Nil)
}
