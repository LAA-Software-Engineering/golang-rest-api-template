package api

import (
	"bytes"
	"context"
	"encoding/json"
	"golang-rest-api-template/pkg/auth"
	"golang-rest-api-template/pkg/database"
	"golang-rest-api-template/pkg/models"
	"net/http"
	"net/http/httptest"
	"testing"

	"gorm.io/gorm"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
)

func TestNewUserRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	mockCtx := context.Background()

	repo := NewUserRepository(mockDB, &mockCtx)

	assert.NotNil(t, repo, "NewUserRepository should return a non-nil instance of userRepository")
	assert.Equal(t, mockDB, repo.DB, "DB should be set to the mock database instance")
}

func TestLoginHandlerSuccess(t *testing.T) {
	// Set up real in-memory DB
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.AutoMigrate(&models.User{})

	repo := NewUserRepository(&database.GormDatabase{DB: db}, nil)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/login", repo.LoginHandler)

	hashedPassword, _ := auth.HashPassword("password")
	user := models.User{Username: "testuser", Password: hashedPassword}
	db.Create(&user)

	loginUser := models.LoginUser{Username: "testuser", Password: "password"}
	requestBody, _ := json.Marshal(loginUser)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "token")
}

func TestLoginHandlerInvalidJSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	ctx := context.Background()
	repo := NewUserRepository(mockDB, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/login", repo.LoginHandler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/login", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestLoginHandlerBindValidationLoginUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	ctx := context.Background()
	repo := NewUserRepository(mockDB, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/login", repo.LoginHandler)

	// Missing password — binding is on models.LoginUser, not models.User
	body, _ := json.Marshal(map[string]string{"username": "onlyuser"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Password")
}

func TestLoginHandlerUserNotFound(t *testing.T) {
	// Set up real in-memory DB
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.AutoMigrate(&models.User{})

	repo := NewUserRepository(&database.GormDatabase{DB: db}, nil)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/login", repo.LoginHandler)

	loginUser := models.LoginUser{Username: "nonexistent", Password: "password"}
	requestBody, _ := json.Marshal(loginUser)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid username or password")
}

func TestLoginHandlerWrongPassword(t *testing.T) {
	// Set up real in-memory DB
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.AutoMigrate(&models.User{})

	repo := NewUserRepository(&database.GormDatabase{DB: db}, nil)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/login", repo.LoginHandler)

	hashedPassword, _ := auth.HashPassword("correctpassword")
	user := models.User{Username: "testuser", Password: hashedPassword}
	db.Create(&user)

	loginUser := models.LoginUser{Username: "testuser", Password: "wrongpassword"}
	requestBody, _ := json.Marshal(loginUser)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid username or password")
}

func TestRegisterHandlerInvalidJSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	ctx := context.Background()
	repo := NewUserRepository(mockDB, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/register", repo.RegisterHandler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/register", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestRegisterHandlerDBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	ctx := context.Background()
	repo := NewUserRepository(mockDB, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/register", repo.RegisterHandler)

	loginUser := models.LoginUser{Username: "newuser", Password: "password"}
	requestBody, _ := json.Marshal(loginUser)

	mockDB.EXPECT().Create(gomock.Any()).Return(&gorm.DB{Error: gorm.ErrInvalidDB})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/register", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Could not save user")
}
