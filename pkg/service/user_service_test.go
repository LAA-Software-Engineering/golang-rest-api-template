package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"golang-rest-api-template/pkg/auth"
	"golang-rest-api-template/pkg/models"
	"golang-rest-api-template/pkg/repository"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type fakeUserStore struct {
	findFn     func(username string) (*models.User, error)
	findByIDFn func(id uint) (*models.User, error)
	createFn   func(user *models.User) error
}

func (f *fakeUserStore) FindByUsername(username string) (*models.User, error) {
	if f.findFn != nil {
		return f.findFn(username)
	}
	return nil, nil
}

func (f *fakeUserStore) FindByID(id uint) (*models.User, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(id)
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeUserStore) Create(user *models.User) error {
	if f.createFn != nil {
		return f.createFn(user)
	}
	return nil
}

type memRefreshStore struct {
	mu     sync.Mutex
	byHash map[string]*models.RefreshToken
	nextID uint
}

func newMemRefreshStore() *memRefreshStore {
	return &memRefreshStore{byHash: make(map[string]*models.RefreshToken)}
}

func (m *memRefreshStore) Create(token *models.RefreshToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	cp := *token
	cp.ID = m.nextID
	m.byHash[cp.TokenHash] = &cp
	return nil
}

func (m *memRefreshStore) FindByHash(tokenHash string) (*models.RefreshToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.byHash[tokenHash]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *t
	return &cp, nil
}

func (m *memRefreshStore) MarkConsumed(id uint, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.byHash {
		if t.ID == id {
			t.ConsumedAt = &at
			return nil
		}
	}
	return gorm.ErrRecordNotFound
}

func (m *memRefreshStore) RevokeFamily(familyID string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.byHash {
		if t.FamilyID == familyID && t.RevokedAt == nil {
			t.RevokedAt = &at
		}
	}
	return nil
}

func (m *memRefreshStore) RevokeAllForUser(userID uint, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.byHash {
		if t.UserID == userID && t.RevokedAt == nil {
			t.RevokedAt = &at
		}
	}
	return nil
}

func testUserService(users repository.UserPersistence, refresh repository.RefreshTokenPersistence) *UserService {
	return NewUserService(users, refresh, auth.NoopDenylist{})
}

func TestUserServiceLoginUserNotFound(t *testing.T) {
	store := &fakeUserStore{
		findFn: func(username string) (*models.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := testUserService(store, newMemRefreshStore())
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
	svc := testUserService(store, newMemRefreshStore())
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
	svc := testUserService(store, newMemRefreshStore())
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
	svc := testUserService(store, newMemRefreshStore())
	pair, err := svc.Login(context.Background(), "alice", "secret")
	assert.NoError(t, err)
	assert.NotEmpty(t, pair.AccessToken)
	assert.NotEmpty(t, pair.RefreshToken)
	assert.Greater(t, pair.ExpiresIn, int64(0))
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
	svc := testUserService(store, newMemRefreshStore())
	pair, err := svc.Login(context.Background(), "boss", "secret")
	assert.NoError(t, err)

	parsed := &auth.Claims{}
	token, err := jwt.ParseWithClaims(pair.AccessToken, parsed, auth.JWTKeyFunc(auth.JWTSigningKey()))
	assert.NoError(t, err)
	assert.True(t, token.Valid)
	assert.Equal(t, auth.RoleAdmin, parsed.Role)
	assert.NotEmpty(t, parsed.ID)
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
	svc := testUserService(store, newMemRefreshStore())
	pair, err := svc.Login(context.Background(), "legacy", "secret")
	assert.NoError(t, err)

	parsed := &auth.Claims{}
	_, err = jwt.ParseWithClaims(pair.AccessToken, parsed, auth.JWTKeyFunc(auth.JWTSigningKey()))
	assert.NoError(t, err)
	assert.Equal(t, auth.RoleUser, parsed.Role)
}

func TestUserServiceRefreshRotationAndReuse(t *testing.T) {
	prev := auth.JWTSigningKey()
	t.Cleanup(func() { _ = auth.SetJWTSigningKey(prev) })
	require.NoError(t, auth.SetJWTSigningKey(bytes.Repeat([]byte("s"), auth.MinJWTSecretKeyBytes)))

	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	require.NoError(t, err)
	user := &models.User{ID: 9, Username: "rot", Password: string(hash), Role: auth.RoleUser}
	users := &fakeUserStore{
		findFn: func(username string) (*models.User, error) {
			return user, nil
		},
		findByIDFn: func(id uint) (*models.User, error) {
			if id == user.ID {
				return user, nil
			}
			return nil, gorm.ErrRecordNotFound
		},
	}
	refresh := newMemRefreshStore()
	svc := testUserService(users, refresh)

	pair, err := svc.Login(context.Background(), "rot", "secret")
	require.NoError(t, err)
	oldRefresh := pair.RefreshToken

	next, err := svc.Refresh(context.Background(), oldRefresh)
	require.NoError(t, err)
	assert.NotEqual(t, oldRefresh, next.RefreshToken)
	assert.NotEmpty(t, next.AccessToken)

	_, err = svc.Refresh(context.Background(), oldRefresh)
	assert.ErrorIs(t, err, ErrRefreshReuse)
}

func TestUserServiceLogoutRevokesRefresh(t *testing.T) {
	prev := auth.JWTSigningKey()
	t.Cleanup(func() { _ = auth.SetJWTSigningKey(prev) })
	require.NoError(t, auth.SetJWTSigningKey(bytes.Repeat([]byte("s"), auth.MinJWTSecretKeyBytes)))

	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	require.NoError(t, err)
	user := &models.User{ID: 11, Username: "out", Password: string(hash), Role: auth.RoleUser}
	users := &fakeUserStore{
		findFn: func(string) (*models.User, error) { return user, nil },
		findByIDFn: func(id uint) (*models.User, error) {
			if id == user.ID {
				return user, nil
			}
			return nil, gorm.ErrRecordNotFound
		},
	}
	refresh := newMemRefreshStore()
	svc := testUserService(users, refresh)

	pair, err := svc.Login(context.Background(), "out", "secret")
	require.NoError(t, err)

	require.NoError(t, svc.Logout(context.Background(), user.ID, "", "jti-1", time.Now().Add(time.Minute)))
	_, err = svc.Refresh(context.Background(), pair.RefreshToken)
	assert.ErrorIs(t, err, ErrInvalidRefresh)
}

type recordingDenylist struct {
	auth.NoopDenylist
	denyUserCalls int
	denyJTICalls  int
	lastUserID    uint
}

func (r *recordingDenylist) Deny(ctx context.Context, jti string, until time.Time) error {
	r.denyJTICalls++
	return nil
}

func (r *recordingDenylist) DenyUserBefore(ctx context.Context, userID uint, before time.Time) error {
	r.denyUserCalls++
	r.lastUserID = userID
	return nil
}

func TestUserServiceLogoutAllCallsDenyUserBefore(t *testing.T) {
	prev := auth.JWTSigningKey()
	t.Cleanup(func() { _ = auth.SetJWTSigningKey(prev) })
	require.NoError(t, auth.SetJWTSigningKey(bytes.Repeat([]byte("s"), auth.MinJWTSecretKeyBytes)))

	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	require.NoError(t, err)
	user := &models.User{ID: 21, Username: "all", Password: string(hash), Role: auth.RoleUser}
	users := &fakeUserStore{
		findFn:     func(string) (*models.User, error) { return user, nil },
		findByIDFn: func(id uint) (*models.User, error) { return user, nil },
	}
	dl := &recordingDenylist{}
	svc := NewUserService(users, newMemRefreshStore(), dl)

	pair, err := svc.Login(context.Background(), "all", "secret")
	require.NoError(t, err)

	require.NoError(t, svc.Logout(context.Background(), user.ID, "", "jti-x", time.Now().Add(time.Minute)))
	assert.Equal(t, 1, dl.denyUserCalls)
	assert.Equal(t, user.ID, dl.lastUserID)
	assert.Equal(t, 1, dl.denyJTICalls)

	_, err = svc.Refresh(context.Background(), pair.RefreshToken)
	assert.ErrorIs(t, err, ErrInvalidRefresh)
}

func TestUserServiceLogoutFamilySkipsDenyUserBefore(t *testing.T) {
	prev := auth.JWTSigningKey()
	t.Cleanup(func() { _ = auth.SetJWTSigningKey(prev) })
	require.NoError(t, auth.SetJWTSigningKey(bytes.Repeat([]byte("s"), auth.MinJWTSecretKeyBytes)))

	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	require.NoError(t, err)
	user := &models.User{ID: 22, Username: "one", Password: string(hash), Role: auth.RoleUser}
	users := &fakeUserStore{
		findFn:     func(string) (*models.User, error) { return user, nil },
		findByIDFn: func(id uint) (*models.User, error) { return user, nil },
	}
	dl := &recordingDenylist{}
	svc := NewUserService(users, newMemRefreshStore(), dl)

	pair, err := svc.Login(context.Background(), "one", "secret")
	require.NoError(t, err)

	require.NoError(t, svc.Logout(context.Background(), user.ID, pair.RefreshToken, "jti-y", time.Now().Add(time.Minute)))
	assert.Equal(t, 0, dl.denyUserCalls)
	assert.Equal(t, 1, dl.denyJTICalls)
}

func TestUserServiceRegisterSetsUserRole(t *testing.T) {
	var created *models.User
	store := &fakeUserStore{
		createFn: func(user *models.User) error {
			created = user
			return nil
		},
	}
	svc := testUserService(store, newMemRefreshStore())
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
	svc := testUserService(store, newMemRefreshStore())
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
	svc := testUserService(store, newMemRefreshStore())
	err := svc.Register(context.Background(), "u", "password123")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrRegisterSave)
}
