package api

import (
	"bytes"
	"encoding/json"
	"golang-rest-api-template/pkg/auth"
	"golang-rest-api-template/pkg/models"
	"golang-rest-api-template/pkg/repository"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

	mockUsers := repository.NewMockUserPersistence(ctrl)

	repo := NewUserRepository(mockUsers)

	assert.NotNil(t, repo, "NewUserRepository should return a non-nil instance of userRepository")
}

func TestLoginHandlerSuccess(t *testing.T) {
	// Set up real in-memory DB
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatal(err)
	}

	repo := NewUserRepository(repository.NewGormUserStore(db))

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

	mockUsers := repository.NewMockUserPersistence(ctrl)
	repo := NewUserRepository(mockUsers)

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

	mockUsers := repository.NewMockUserPersistence(ctrl)
	repo := NewUserRepository(mockUsers)

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
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatal(err)
	}

	repo := NewUserRepository(repository.NewGormUserStore(db))

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
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatal(err)
	}

	repo := NewUserRepository(repository.NewGormUserStore(db))

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

	mockUsers := repository.NewMockUserPersistence(ctrl)
	repo := NewUserRepository(mockUsers)

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

	mockUsers := repository.NewMockUserPersistence(ctrl)
	repo := NewUserRepository(mockUsers)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/register", repo.RegisterHandler)

	loginUser := models.LoginUser{Username: "newuser", Password: "password"}
	requestBody, _ := json.Marshal(loginUser)

	mockUsers.EXPECT().Create(gomock.Any()).Return(gorm.ErrInvalidDB)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/register", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "Could not save user")
	assert.NotContains(t, body, "ErrInvalidDB")
}

func TestRegisterHandlerDuplicateUsername(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "register_dup.sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatal(err)
	}

	repo := NewUserRepository(repository.NewGormUserStore(db))
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/register", repo.RegisterHandler)

	login := models.LoginUser{Username: "alice", Password: "hunter2!aa"}
	payload, err := json.Marshal(login)
	if err != nil {
		t.Fatal(err)
	}

	req := func() *http.Request {
		req, err := http.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(payload))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		return req
	}

	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req())
	assert.Equal(t, http.StatusCreated, w1.Code)

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req())
	assert.Equal(t, http.StatusConflict, w2.Code)
	assert.Contains(t, w2.Body.String(), "username already taken")
}
