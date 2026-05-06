package httpresp

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type envelope[T any] struct {
	Data T `json:"data"`
}

// OK writes HTTP 200 with the standard JSON envelope {"data": <payload>}.
func OK[T any](c *gin.Context, data T) {
	if c == nil {
		return
	}
	c.JSON(http.StatusOK, envelope[T]{Data: data})
}

// Created writes HTTP 201 with the standard JSON envelope {"data": <payload>}.
func Created[T any](c *gin.Context, data T) {
	if c == nil {
		return
	}
	c.JSON(http.StatusCreated, envelope[T]{Data: data})
}
