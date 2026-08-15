package models

import "time"

type LoginUser struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginTokenBody is the "data" object returned by POST /login and POST /refresh on success.
// Token is kept as an alias of AccessToken for backward compatibility with older clients.
type LoginTokenBody struct {
	Token        string `json:"token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// RefreshRequest is the JSON body for POST /refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// LogoutRequest is the optional JSON body for POST /logout.
// When RefreshToken is set, only that token family is revoked; otherwise all
// refresh tokens for the authenticated user are revoked.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// LogoutSuccessBody is the "data" object returned by POST /logout on success.
type LogoutSuccessBody struct {
	Message string `json:"message"`
}

// RegisterSuccessBody is the "data" object returned by POST /register on success.
type RegisterSuccessBody struct {
	Message string `json:"message"`
}

// AdminMeBody is the "data" object returned by GET /admin/me on success.
type AdminMeBody struct {
	Username string `json:"username"`
	UserID   uint   `json:"user_id"`
	Role     string `json:"role"`
}

type User struct {
	ID        uint      `json:"id" gorm:"primary_key"`
	Username  string    `json:"username" gorm:"unique"`
	Password  string    `json:"-"`
	Role      string    `json:"role" gorm:"type:varchar(32);not null;default:'user'"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}
