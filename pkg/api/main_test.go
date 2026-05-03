package api

import (
	"bytes"

	"golang-rest-api-template/pkg/auth"
	"golang-rest-api-template/pkg/middleware"

	"github.com/gin-gonic/gin"
)

// testAPISecretKey is the X-API-Key value expected by middleware in tests (32 bytes).
const testAPISecretKey = "01234567890123456789012345678901"

func init() {
	// Runs for the full package test binary and whenever this file is linked (e.g. with
	// `go test main_test.go other_test.go`); configures JWT + API key before any test.
	gin.SetMode(gin.TestMode)
	if err := auth.SetJWTSigningKey(bytes.Repeat([]byte("t"), auth.MinJWTSecretKeyBytes)); err != nil {
		panic(err)
	}
	if err := middleware.SetAPISecretKey([]byte(testAPISecretKey)); err != nil {
		panic(err)
	}
}
