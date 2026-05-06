package models

import "time"

type Book struct {
	ID        uint      `json:"id" gorm:"primary_key"`
	OwnerID   uint      `json:"owner_id" gorm:"index;not null"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

type CreateBook struct {
	Title  string `json:"title" binding:"required"`
	Author string `json:"author" binding:"required"`
}

type UpdateBook struct {
	Title  string `json:"title"`
	Author string `json:"author"`
}
