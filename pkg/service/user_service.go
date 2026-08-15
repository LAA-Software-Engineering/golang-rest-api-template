package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang-rest-api-template/pkg/auth"
	"golang-rest-api-template/pkg/models"
	"golang-rest-api-template/pkg/repository"

	"golang.org/x/crypto/bcrypt"
)

// Sentinel errors for login / registration / token lifecycle.
var (
	ErrInvalidLogin     = errors.New("service: invalid username or password")
	ErrLoginDB          = errors.New("service: login database error")
	ErrTokenGenerate    = errors.New("service: token generation failed")
	ErrRegisterConflict = errors.New("service: username already taken")
	ErrRegisterHash     = errors.New("service: password hash failed")
	ErrRegisterSave     = errors.New("service: could not save user")
	ErrInvalidRefresh   = errors.New("service: invalid refresh token")
	ErrRefreshReuse     = errors.New("service: refresh token reuse detected")
	ErrRefreshPersist   = errors.New("service: refresh token persistence failed")
	ErrLogoutPersist    = errors.New("service: logout persistence failed")
)

// TokenPair is the access + refresh credential pair returned by login/refresh.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

// UserService handles authentication use-cases without Gin.
type UserService struct {
	users    repository.UserPersistence
	refresh  repository.RefreshTokenPersistence
	denylist auth.TokenDenylist
}

// NewUserService constructs a UserService. refresh and denylist may be nil only
// for register-only tests; Login/Refresh/Logout require a non-nil refresh store.
func NewUserService(users repository.UserPersistence, refresh repository.RefreshTokenPersistence, denylist auth.TokenDenylist) *UserService {
	if denylist == nil {
		denylist = auth.NoopDenylist{}
	}
	return &UserService{users: users, refresh: refresh, denylist: denylist}
}

// Login validates credentials and returns an access JWT plus opaque refresh token.
func (s *UserService) Login(ctx context.Context, username, password string) (TokenPair, error) {
	var zero TokenPair
	dbUser, err := s.users.FindByUsername(username)
	if err != nil {
		if repository.IsUserNotFound(err) {
			return zero, ErrInvalidLogin
		}
		return zero, fmtError(ErrLoginDB, err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(dbUser.Password), []byte(password)); err != nil {
		return zero, ErrInvalidLogin
	}
	return s.issueTokenPair(ctx, dbUser)
}

// Refresh rotates a refresh token and returns a new access + refresh pair.
// Reuse of a consumed token revokes the entire family.
func (s *UserService) Refresh(ctx context.Context, refreshPlaintext string) (TokenPair, error) {
	var zero TokenPair
	if s.refresh == nil {
		return zero, ErrRefreshPersist
	}
	if refreshPlaintext == "" {
		return zero, ErrInvalidRefresh
	}

	hash := auth.HashRefreshToken(refreshPlaintext)
	row, err := s.refresh.FindByHash(hash)
	if err != nil {
		if repository.IsUserNotFound(err) {
			return zero, ErrInvalidRefresh
		}
		return zero, fmtError(ErrRefreshPersist, err)
	}

	now := time.Now()
	if row.RevokedAt != nil {
		return zero, ErrInvalidRefresh
	}
	if row.ExpiresAt.Before(now) {
		return zero, ErrInvalidRefresh
	}
	if row.ConsumedAt != nil {
		_ = s.refresh.RevokeFamily(row.FamilyID, now)
		return zero, ErrRefreshReuse
	}

	if err := s.refresh.MarkConsumed(row.ID, now); err != nil {
		return zero, fmtError(ErrRefreshPersist, err)
	}

	dbUser, err := s.users.FindByID(row.UserID)
	if err != nil {
		if repository.IsUserNotFound(err) {
			return zero, ErrInvalidRefresh
		}
		return zero, fmtError(ErrLoginDB, err)
	}
	return s.issueTokenPairWithFamily(ctx, dbUser, row.FamilyID)
}

// Logout revokes refresh tokens and optionally denylists the current access jti.
// If refreshPlaintext is non-empty, only that token's family is revoked; otherwise
// all refresh tokens for userID are revoked.
func (s *UserService) Logout(ctx context.Context, userID uint, refreshPlaintext, accessJTI string, accessExp time.Time) error {
	if s.refresh == nil {
		return ErrLogoutPersist
	}
	now := time.Now()

	if refreshPlaintext != "" {
		hash := auth.HashRefreshToken(refreshPlaintext)
		row, err := s.refresh.FindByHash(hash)
		if err != nil {
			if !repository.IsUserNotFound(err) {
				return fmtError(ErrLogoutPersist, err)
			}
			// Unknown refresh: still denylist access token and succeed.
		} else if row.UserID == userID {
			if err := s.refresh.RevokeFamily(row.FamilyID, now); err != nil {
				return fmtError(ErrLogoutPersist, err)
			}
		}
	} else {
		if err := s.refresh.RevokeAllForUser(userID, now); err != nil {
			return fmtError(ErrLogoutPersist, err)
		}
		if err := s.denylist.DenyUserBefore(ctx, userID, now); err != nil {
			// User revoke_before is best-effort; refresh revocation already succeeded.
			_ = err
		}
	}

	if accessJTI != "" && !accessExp.IsZero() {
		if err := s.denylist.Deny(ctx, accessJTI, accessExp); err != nil {
			// Denylist is best-effort; refresh revocation already succeeded.
			_ = err
		}
	}
	return nil
}

// Register creates a new user account.
func (s *UserService) Register(_ context.Context, username, password string) error {
	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		return fmtError(ErrRegisterHash, err)
	}
	newUser := &models.User{Username: username, Password: hashedPassword, Role: auth.RoleUser}
	if err := s.users.Create(newUser); err != nil {
		if errors.Is(err, repository.ErrUserUsernameConflict) {
			return ErrRegisterConflict
		}
		return fmtError(ErrRegisterSave, err)
	}
	return nil
}

func (s *UserService) issueTokenPair(ctx context.Context, dbUser *models.User) (TokenPair, error) {
	familyID, err := auth.NewFamilyID()
	if err != nil {
		return TokenPair{}, fmtError(ErrTokenGenerate, err)
	}
	return s.issueTokenPairWithFamily(ctx, dbUser, familyID)
}

func (s *UserService) issueTokenPairWithFamily(_ context.Context, dbUser *models.User, familyID string) (TokenPair, error) {
	var zero TokenPair
	role, err := auth.EffectiveRole(dbUser.Role)
	if err != nil {
		return zero, fmtError(ErrTokenGenerate, err)
	}
	access, err := auth.GenerateToken(dbUser.Username, dbUser.ID, role)
	if err != nil {
		return zero, fmtError(ErrTokenGenerate, err)
	}

	plain, err := auth.NewOpaqueToken()
	if err != nil {
		return zero, fmtError(ErrTokenGenerate, err)
	}
	if s.refresh == nil {
		return zero, ErrRefreshPersist
	}
	row := &models.RefreshToken{
		UserID:    dbUser.ID,
		TokenHash: auth.HashRefreshToken(plain),
		FamilyID:  familyID,
		ExpiresAt: time.Now().Add(auth.RefreshTokenTTL()),
	}
	if err := s.refresh.Create(row); err != nil {
		return zero, fmtError(ErrRefreshPersist, err)
	}

	return TokenPair{
		AccessToken:  access,
		RefreshToken: plain,
		ExpiresIn:    int64(auth.AccessTokenTTL().Seconds()),
	}, nil
}

func fmtError(sentinel error, cause error) error {
	return fmt.Errorf("%w: %v", sentinel, cause)
}
