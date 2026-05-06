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

// ReplaceBook is the JSON body for PUT /books/:id (full replace of title and author).
type ReplaceBook struct {
	Title  string `json:"title" binding:"required"`
	Author string `json:"author" binding:"required"`
}

// PatchBook is the JSON body for PATCH /books/:id. Omitted JSON keys leave the field
// unchanged; the request must set at least one of title or author.
type PatchBook struct {
	Title  *string `json:"title,omitempty"`
	Author *string `json:"author,omitempty"`
}
