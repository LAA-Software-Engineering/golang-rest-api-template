package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang-rest-api-template/pkg/auth"
	"golang-rest-api-template/pkg/models"
	"golang-rest-api-template/pkg/repository"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Sentinel errors for login / registration.
var (
	ErrInvalidLogin     = errors.New("service: invalid username or password")
	ErrLoginDB          = errors.New("service: login database error")
	ErrTokenGenerate    = errors.New("service: token generation failed")
	ErrRegisterConflict = errors.New("service: username already taken")
	ErrRegisterHash     = errors.New("service: password hash failed")
	ErrRegisterSave     = errors.New("service: could not save user")
)

// UserService handles authentication use-cases without Gin.
type UserService struct {
	users repository.UserPersistence
}

// NewUserService constructs a UserService.
func NewUserService(users repository.UserPersistence) *UserService {
	return &UserService{users: users}
}

// Login validates credentials and returns a JWT token string.
func (s *UserService) Login(_ context.Context, username, password string) (token string, err error) {
	dbUser, err := s.users.FindByUsername(username)
	if err != nil {
		if repository.IsUserNotFound(err) {
			return "", ErrInvalidLogin
		}
		return "", fmtError(ErrLoginDB, err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(dbUser.Password), []byte(password)); err != nil {
		return "", ErrInvalidLogin
	}
	tok, err := auth.GenerateToken(dbUser.Username, dbUser.ID)
	if err != nil {
		return "", fmtError(ErrTokenGenerate, err)
	}
	return tok, nil
}

// Register creates a new user account.
func (s *UserService) Register(_ context.Context, username, password string) error {
	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		return fmtError(ErrRegisterHash, err)
	}
	newUser := &models.User{Username: username, Password: hashedPassword}
	if err := s.users.Create(newUser); err != nil {
		if registerUsernameConflict(err) {
			return ErrRegisterConflict
		}
		return fmtError(ErrRegisterSave, err)
	}
	return nil
}

func fmtError(sentinel error, cause error) error {
	return fmt.Errorf("%w: %v", sentinel, cause)
}

func registerUsernameConflict(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "unique constraint failed") {
		return true
	}
	if strings.Contains(s, "duplicate key") && strings.Contains(s, "unique") {
		return true
	}
	return false
}
