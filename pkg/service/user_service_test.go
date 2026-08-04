package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"golang-rest-api-template/pkg/auth"
	"golang-rest-api-template/pkg/models"
	"golang-rest-api-template/pkg/repository"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type fakeUserStore struct {
	findFn   func(username string) (*models.User, error)
	createFn func(user *models.User) error
}

func (f *fakeUserStore) FindByUsername(username string) (*models.User, error) {
	if f.findFn != nil {
		return f.findFn(username)
	}
	return nil, nil
}

func (f *fakeUserStore) Create(user *models.User) error {
	if f.createFn != nil {
		return f.createFn(user)
	}
	return nil
}

func TestUserServiceLoginUserNotFound(t *testing.T) {
	store := &fakeUserStore{
		findFn: func(username string) (*models.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewUserService(store)
	_, err := svc.Login(context.Background(), "nobody", "pw")
	assert.ErrorIs(t, err, ErrInvalidLogin)
}

func TestUserServiceLoginWrongPassword(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("right"), bcrypt.MinCost)
	assert.NoError(t, err)
	store := &fakeUserStore{
		findFn: func(username string) (*models.User, error) {
			return &models.User{Username: "u", Password: string(hash)}, nil
		},
	}
	svc := NewUserService(store)
	_, err = svc.Login(context.Background(), "u", "wrong")
	assert.ErrorIs(t, err, ErrInvalidLogin)
}

func TestUserServiceLoginDBError(t *testing.T) {
	dbErr := errors.New("connection refused")
	store := &fakeUserStore{
		findFn: func(username string) (*models.User, error) {
			return nil, dbErr
		},
	}
	svc := NewUserService(store)
	_, err := svc.Login(context.Background(), "u", "p")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrLoginDB)
}

func TestUserServiceLoginSuccess(t *testing.T) {
	prev := auth.JWTSigningKey()
	t.Cleanup(func() { _ = auth.SetJWTSigningKey(prev) })
	assert.NoError(t, auth.SetJWTSigningKey(bytes.Repeat([]byte("s"), auth.MinJWTSecretKeyBytes)))

	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	assert.NoError(t, err)
	store := &fakeUserStore{
		findFn: func(username string) (*models.User, error) {
			return &models.User{ID: 100, Username: "alice", Password: string(hash), Role: auth.RoleUser}, nil
		},
	}
	svc := NewUserService(store)
	tok, err := svc.Login(context.Background(), "alice", "secret")
	assert.NoError(t, err)
	assert.NotEmpty(t, tok)
}

func TestUserServiceLoginEmbedsAdminRole(t *testing.T) {
	prev := auth.JWTSigningKey()
	t.Cleanup(func() { _ = auth.SetJWTSigningKey(prev) })
	assert.NoError(t, auth.SetJWTSigningKey(bytes.Repeat([]byte("s"), auth.MinJWTSecretKeyBytes)))

	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	assert.NoError(t, err)
	store := &fakeUserStore{
		findFn: func(username string) (*models.User, error) {
			return &models.User{ID: 3, Username: "boss", Password: string(hash), Role: auth.RoleAdmin}, nil
		},
	}
	svc := NewUserService(store)
	tok, err := svc.Login(context.Background(), "boss", "secret")
	assert.NoError(t, err)

	parsed := &auth.Claims{}
	token, err := jwt.ParseWithClaims(tok, parsed, auth.JWTKeyFunc(auth.JWTSigningKey()))
	assert.NoError(t, err)
	assert.True(t, token.Valid)
	assert.Equal(t, auth.RoleAdmin, parsed.Role)
}

func TestUserServiceLoginEmptyRoleDefaultsToUser(t *testing.T) {
	prev := auth.JWTSigningKey()
	t.Cleanup(func() { _ = auth.SetJWTSigningKey(prev) })
	assert.NoError(t, auth.SetJWTSigningKey(bytes.Repeat([]byte("s"), auth.MinJWTSecretKeyBytes)))

	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	assert.NoError(t, err)
	store := &fakeUserStore{
		findFn: func(username string) (*models.User, error) {
			return &models.User{ID: 4, Username: "legacy", Password: string(hash), Role: ""}, nil
		},
	}
	svc := NewUserService(store)
	tok, err := svc.Login(context.Background(), "legacy", "secret")
	assert.NoError(t, err)

	parsed := &auth.Claims{}
	_, err = jwt.ParseWithClaims(tok, parsed, auth.JWTKeyFunc(auth.JWTSigningKey()))
	assert.NoError(t, err)
	assert.Equal(t, auth.RoleUser, parsed.Role)
}

func TestUserServiceRegisterSetsUserRole(t *testing.T) {
	var created *models.User
	store := &fakeUserStore{
		createFn: func(user *models.User) error {
			created = user
			return nil
		},
	}
	svc := NewUserService(store)
	err := svc.Register(context.Background(), "newbie", "password123")
	assert.NoError(t, err)
	if assert.NotNil(t, created) {
		assert.Equal(t, auth.RoleUser, created.Role)
	}
}

func TestUserServiceRegisterConflictDuplicatedKey(t *testing.T) {
	store := &fakeUserStore{
		createFn: func(user *models.User) error {
			return fmt.Errorf("%w: %v", repository.ErrUserUsernameConflict, gorm.ErrDuplicatedKey)
		},
	}
	svc := NewUserService(store)
	err := svc.Register(context.Background(), "dup", "password123")
	assert.ErrorIs(t, err, ErrRegisterConflict)
}

func TestUserServiceRegisterSaveError(t *testing.T) {
	saveErr := errors.New("disk full")
	store := &fakeUserStore{
		createFn: func(user *models.User) error {
			return saveErr
		},
	}
	svc := NewUserService(store)
	err := svc.Register(context.Background(), "u", "password123")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrRegisterSave)
}
