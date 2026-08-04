package models

import "time"

type LoginUser struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginTokenBody is the "data" object returned by POST /login on success.
type LoginTokenBody struct {
	Token string `json:"token"`
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
