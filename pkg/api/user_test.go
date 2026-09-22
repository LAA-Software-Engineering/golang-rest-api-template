package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang-rest-api-template/internal/pgtest"
	"golang-rest-api-template/pkg/auth"
	"golang-rest-api-template/pkg/middleware"
	"golang-rest-api-template/pkg/models"
	"golang-rest-api-template/pkg/repository"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func openUserTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return pgtest.Pool(t)
}

func newTestUserHandler(pool *pgxpool.Pool) *userHandler {
	return NewUserHandler(
		repository.NewSQLCUserStore(pool),
		repository.NewSQLCRefreshTokenStore(pool),
		auth.NoopDenylist{},
	)
}

// seedUser inserts a user with the given plaintext password (hashed) directly via
// the persistence layer.
func seedUser(t *testing.T, pool *pgxpool.Pool, username, plaintext string) {
	t.Helper()
	hashed, err := auth.HashPassword(plaintext)
	require.NoError(t, err)
	require.NoError(t, repository.NewSQLCUserStore(pool).Create(context.Background(), &models.User{
		Username: username,
		Password: hashed,
		Role:     auth.RoleUser,
	}))
}

func TestNewUserHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUsers := repository.NewMockUserPersistence(ctrl)
	h := NewUserHandler(mockUsers, nil, auth.NoopDenylist{})
	assert.NotNil(t, h, "NewUserHandler should return a non-nil *userHandler")
}

func TestLoginHandlerSuccess(t *testing.T) {
	prev := auth.JWTSigningKey()
	t.Cleanup(func() { _ = auth.SetJWTSigningKey(prev) })
	require.NoError(t, auth.SetJWTSigningKey(bytes.Repeat([]byte("k"), auth.MinJWTSecretKeyBytes)))

	pool := openUserTestDB(t)
	h := newTestUserHandler(pool)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/login", h.LoginHandler)

	seedUser(t, pool, "testuser", "password")

	loginUser := models.LoginUser{Username: "testuser", Password: "password"}
	requestBody, _ := json.Marshal(loginUser)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var loginBody struct {
		Data models.LoginTokenBody `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &loginBody))
	assert.NotEmpty(t, loginBody.Data.Token)
	assert.Equal(t, loginBody.Data.Token, loginBody.Data.AccessToken)
	assert.NotEmpty(t, loginBody.Data.RefreshToken)
	assert.Greater(t, loginBody.Data.ExpiresIn, int64(0))
}

func TestLoginHandlerInvalidJSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUsers := repository.NewMockUserPersistence(ctrl)
	h := NewUserHandler(mockUsers, nil, auth.NoopDenylist{})

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/login", h.LoginHandler)

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
	h := NewUserHandler(mockUsers, nil, auth.NoopDenylist{})

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/login", h.LoginHandler)

	body, _ := json.Marshal(map[string]string{"username": "onlyuser"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Password")
}

func TestLoginHandlerUserNotFound(t *testing.T) {
	pool := openUserTestDB(t)
	h := newTestUserHandler(pool)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/login", h.LoginHandler)

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
	pool := openUserTestDB(t)
	h := newTestUserHandler(pool)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/login", h.LoginHandler)

	seedUser(t, pool, "testuser", "correctpassword")

	loginUser := models.LoginUser{Username: "testuser", Password: "wrongpassword"}
	requestBody, _ := json.Marshal(loginUser)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid username or password")
}

func TestRefreshAndLogoutHandlers(t *testing.T) {
	prev := auth.JWTSigningKey()
	t.Cleanup(func() { _ = auth.SetJWTSigningKey(prev) })
	require.NoError(t, auth.SetJWTSigningKey(bytes.Repeat([]byte("k"), auth.MinJWTSecretKeyBytes)))

	pool := openUserTestDB(t)
	h := newTestUserHandler(pool)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/login", h.LoginHandler)
	r.POST("/refresh", h.RefreshHandler)
	r.POST("/logout", middleware.JWTAuth(auth.NoopDenylist{}), h.LogoutHandler)

	seedUser(t, pool, "refreshuser", "password")

	loginBody, _ := json.Marshal(models.LoginUser{Username: "refreshuser", Password: "password"})
	wLogin := httptest.NewRecorder()
	reqLogin, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(loginBody))
	reqLogin.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wLogin, reqLogin)
	require.Equal(t, http.StatusOK, wLogin.Code)

	var loginResp struct {
		Data models.LoginTokenBody `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wLogin.Body.Bytes(), &loginResp))

	refreshBody, _ := json.Marshal(models.RefreshRequest{RefreshToken: loginResp.Data.RefreshToken})
	wRefresh := httptest.NewRecorder()
	reqRefresh, _ := http.NewRequest("POST", "/refresh", bytes.NewBuffer(refreshBody))
	reqRefresh.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wRefresh, reqRefresh)
	require.Equal(t, http.StatusOK, wRefresh.Code)

	var refreshResp struct {
		Data models.LoginTokenBody `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wRefresh.Body.Bytes(), &refreshResp))
	assert.NotEqual(t, loginResp.Data.RefreshToken, refreshResp.Data.RefreshToken)

	wLogout := httptest.NewRecorder()
	reqLogout, _ := http.NewRequest("POST", "/logout", bytes.NewBuffer([]byte(`{}`)))
	reqLogout.Header.Set("Content-Type", "application/json")
	reqLogout.Header.Set("Authorization", "Bearer "+refreshResp.Data.AccessToken)
	r.ServeHTTP(wLogout, reqLogout)
	assert.Equal(t, http.StatusOK, wLogout.Code)

	wRefresh2 := httptest.NewRecorder()
	reuseBody, _ := json.Marshal(models.RefreshRequest{RefreshToken: refreshResp.Data.RefreshToken})
	reqReuse, _ := http.NewRequest("POST", "/refresh", bytes.NewBuffer(reuseBody))
	reqReuse.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wRefresh2, reqReuse)
	assert.Equal(t, http.StatusUnauthorized, wRefresh2.Code)
}

func TestRegisterHandlerInvalidJSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUsers := repository.NewMockUserPersistence(ctrl)
	h := NewUserHandler(mockUsers, nil, auth.NoopDenylist{})

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/register", h.RegisterHandler)

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
	h := NewUserHandler(mockUsers, nil, auth.NoopDenylist{})

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/register", h.RegisterHandler)

	loginUser := models.LoginUser{Username: "newuser", Password: "password"}
	requestBody, _ := json.Marshal(loginUser)

	mockUsers.EXPECT().Create(gomock.Any(), gomock.Any()).Return(errors.New("internal-db-failure"))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/register", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "Could not save user")
	assert.NotContains(t, body, "internal-db-failure")
}

func TestRegisterHandlerDuplicateUsername(t *testing.T) {
	pool := openUserTestDB(t)

	h := NewUserHandler(repository.NewSQLCUserStore(pool), nil, auth.NoopDenylist{})
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/register", h.RegisterHandler)

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

func TestRegisterHandlerPasswordTooLong(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUsers := repository.NewMockUserPersistence(ctrl)
	h := NewUserHandler(mockUsers, nil, auth.NoopDenylist{})

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/register", h.RegisterHandler)

	loginUser := models.LoginUser{
		Username: "newuser",
		Password: string(bytes.Repeat([]byte("a"), 73)),
	}
	requestBody, _ := json.Marshal(loginUser)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/register", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "password must be at most 72 bytes")
}

