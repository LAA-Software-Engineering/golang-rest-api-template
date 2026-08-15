package api

import (
	"errors"
	"net/http"
	"time"

	"golang-rest-api-template/pkg/auth"
	"golang-rest-api-template/pkg/httperr"
	"golang-rest-api-template/pkg/httpresp"
	"golang-rest-api-template/pkg/middleware"
	"golang-rest-api-template/pkg/models"
	"golang-rest-api-template/pkg/repository"
	"golang-rest-api-template/pkg/service"

	"github.com/gin-gonic/gin"
)

// UserHandler defines Gin handlers for auth routes (HTTP layer only).
type UserHandler interface {
	LoginHandler(c *gin.Context)
	RegisterHandler(c *gin.Context)
	RefreshHandler(c *gin.Context)
	LogoutHandler(c *gin.Context)
	AdminMeHandler(c *gin.Context)
}

type userHandler struct {
	svc *service.UserService
}

// NewUserHandler wires persistence and denylist into user HTTP handlers.
func NewUserHandler(users repository.UserPersistence, refresh repository.RefreshTokenPersistence, denylist auth.TokenDenylist) *userHandler {
	return &userHandler{svc: service.NewUserService(users, refresh, denylist)}
}

func tokenPairBody(p service.TokenPair) models.LoginTokenBody {
	return models.LoginTokenBody{
		Token:        p.AccessToken,
		AccessToken:  p.AccessToken,
		RefreshToken: p.RefreshToken,
		ExpiresIn:    p.ExpiresIn,
	}
}

// @BasePath /api/v1

// LoginHandler godoc
// @Summary Authenticate a user
// @Schemes
// @Description Authenticates a user using username and password; returns access JWT and opaque refresh token
// @Tags user
// @Security ApiKeyAuth
// @Accept  json
// @Produce  json
// @Param   user     body    models.LoginUser     true        "User login object"
// @Success 200 {object} models.LoginAPIResponse "Tokens in standard envelope"
// @Failure 400 {string} string "Bad Request"
// @Failure 401 {string} string "Unauthorized"
// @Failure 500 {string} string "Internal Server Error"
// @Router /login [post]
func (h *userHandler) LoginHandler(c *gin.Context) {
	var incoming models.LoginUser
	if err := c.ShouldBindJSON(&incoming); err != nil {
		httperr.Write(c, http.StatusBadRequest, err.Error())
		return
	}
	pair, err := h.svc.Login(c.Request.Context(), incoming.Username, incoming.Password)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidLogin):
			httperr.Write(c, http.StatusUnauthorized, "Invalid username or password")
		case errors.Is(err, service.ErrLoginDB), errors.Is(err, service.ErrTokenGenerate), errors.Is(err, service.ErrRefreshPersist):
			httperr.Write(c, http.StatusInternalServerError, "Internal Server Error")
		default:
			httperr.Write(c, http.StatusInternalServerError, "Internal Server Error")
		}
		return
	}
	httpresp.OK(c, tokenPairBody(pair))
}

// RefreshHandler godoc
// @Summary Refresh access token
// @Schemes
// @Description Exchanges a valid refresh token for a new access JWT and rotated refresh token
// @Tags user
// @Security ApiKeyAuth
// @Accept  json
// @Produce  json
// @Param   body  body  models.RefreshRequest  true  "Refresh token"
// @Success 200 {object} models.RefreshAPIResponse "New tokens in standard envelope"
// @Failure 400 {string} string "Bad Request"
// @Failure 401 {string} string "Unauthorized"
// @Failure 500 {string} string "Internal Server Error"
// @Router /refresh [post]
func (h *userHandler) RefreshHandler(c *gin.Context) {
	var incoming models.RefreshRequest
	if err := c.ShouldBindJSON(&incoming); err != nil {
		httperr.Write(c, http.StatusBadRequest, err.Error())
		return
	}
	pair, err := h.svc.Refresh(c.Request.Context(), incoming.RefreshToken)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidRefresh), errors.Is(err, service.ErrRefreshReuse):
			httperr.Write(c, http.StatusUnauthorized, "Invalid refresh token")
		case errors.Is(err, service.ErrRefreshPersist), errors.Is(err, service.ErrTokenGenerate), errors.Is(err, service.ErrLoginDB):
			httperr.Write(c, http.StatusInternalServerError, "Internal Server Error")
		default:
			httperr.Write(c, http.StatusInternalServerError, "Internal Server Error")
		}
		return
	}
	httpresp.OK(c, tokenPairBody(pair))
}

// LogoutHandler godoc
// @Summary Log out and revoke tokens
// @Schemes
// @Description Revokes refresh token(s) for the authenticated user and denylists the current access JWT when Redis denylist is enabled
// @Tags user
// @Security ApiKeyAuth
// @Security JwtAuth
// @Accept  json
// @Produce  json
// @Param   body  body  models.LogoutRequest  false  "Optional refresh token to revoke a single family"
// @Success 200 {object} models.LogoutAPIResponse "Logout confirmation"
// @Failure 401 {string} string "Unauthorized"
// @Failure 500 {string} string "Internal Server Error"
// @Router /logout [post]
func (h *userHandler) LogoutHandler(c *gin.Context) {
	var incoming models.LogoutRequest
	// Empty body is allowed; bind errors only when body is present but invalid JSON.
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&incoming); err != nil {
			httperr.Write(c, http.StatusBadRequest, err.Error())
			return
		}
	}

	userID, _ := c.Get(middleware.ContextUserID)
	uid, _ := userID.(uint)
	jtiVal, _ := c.Get(middleware.ContextJTI)
	jti, _ := jtiVal.(string)
	expVal, _ := c.Get(middleware.ContextAccessExp)
	exp, _ := expVal.(time.Time)

	if err := h.svc.Logout(c.Request.Context(), uid, incoming.RefreshToken, jti, exp); err != nil {
		httperr.Write(c, http.StatusInternalServerError, "Internal Server Error")
		return
	}
	httpresp.OK(c, models.LogoutSuccessBody{Message: "Logged out"})
}

// RegisterHandler godoc
// @Summary Register a new user
// @Schemes http
// @Description Registers a new user with the given username and password
// @Tags user
// @Security ApiKeyAuth
// @Accept  json
// @Produce  json
// @Param   user     body    models.LoginUser     true        "User registration object"
// @Success 201 {object} models.RegisterAPIResponse "Registration message in standard envelope"
// @Failure 400 {string} string "Bad Request"
// @Failure 409 {string} string "Conflict"
// @Failure 500 {string} string "Internal Server Error"
// @Router /register [post]
func (h *userHandler) RegisterHandler(c *gin.Context) {
	var user models.LoginUser
	if err := c.ShouldBindJSON(&user); err != nil {
		httperr.Write(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.Register(c.Request.Context(), user.Username, user.Password); err != nil {
		switch {
		case errors.Is(err, service.ErrRegisterConflict):
			httperr.Write(c, http.StatusConflict, "username already taken")
		case errors.Is(err, service.ErrRegisterHash), errors.Is(err, service.ErrRegisterSave):
			httperr.Write(c, http.StatusInternalServerError, "Could not save user")
		default:
			httperr.Write(c, http.StatusInternalServerError, "Could not save user")
		}
		return
	}
	httpresp.Created(c, models.RegisterSuccessBody{Message: "Registration successful"})
}

// AdminMeHandler godoc
// @Summary Current admin identity
// @Schemes
// @Description Returns the authenticated admin's username, user id, and role from JWT claims. Example admin-only route for RBAC.
// @Tags admin
// @Security ApiKeyAuth
// @Security JwtAuth
// @Produce json
// @Success 200 {object} models.AdminMeAPIResponse "Admin identity in standard envelope"
// @Failure 401 {string} string "Unauthorized"
// @Failure 403 {string} string "Forbidden"
// @Router /admin/me [get]
func (h *userHandler) AdminMeHandler(c *gin.Context) {
	username, _ := c.Get("username")
	userID, _ := c.Get(middleware.ContextUserID)
	role, _ := c.Get(middleware.ContextRole)
	uid, _ := userID.(uint)
	uname, _ := username.(string)
	r, _ := role.(string)
	httpresp.OK(c, models.AdminMeBody{
		Username: uname,
		UserID:   uid,
		Role:     r,
	})
}
