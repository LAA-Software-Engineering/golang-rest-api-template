package repository

import "errors"

// ErrUserUsernameConflict is returned by UserPersistence.Create when the insert
// violates the unique username constraint. Callers should use errors.Is
// instead of matching driver-specific strings.
var ErrUserUsernameConflict = errors.New("repository: username already taken")
