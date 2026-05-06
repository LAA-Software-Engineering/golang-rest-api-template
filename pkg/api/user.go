package api

import (
	"errors"
	"net/http"

	"golang-rest-api-template/pkg/httperr"
	"golang-rest-api-template/pkg/models"
	"golang-rest-api-template/pkg/repository"
	"golang-rest-api-template/pkg/service"

	"github.com/gin-gonic/gin"
)

// UserHandler defines Gin handlers for auth routes (HTTP layer only).
type UserHandler interface {
	LoginHandler(c *gin.Context)
	RegisterHandler(c *gin.Context)
}

type userHandler struct {
	svc *service.UserService
}

// NewUserHandler wires persistence into user HTTP handlers.
func NewUserHandler(store repository.UserPersistence) *userHandler {
	return &userHandler{svc: service.NewUserService(store)}
}

// @BasePath /api/v1

// LoginHandler godoc
// @Summary Authenticate a user
// @Schemes
// @Description Authenticates a user using username and password, returns a JWT token if successful
// @Tags user
// @Security ApiKeyAuth
// @Accept  json
// @Produce  json
// @Param   user     body    models.LoginUser     true        "User login object"
// @Success 200 {string} string "JWT Token"
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
	token, err := h.svc.Login(c.Request.Context(), incoming.Username, incoming.Password)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidLogin):
			httperr.Write(c, http.StatusUnauthorized, "Invalid username or password")
		case errors.Is(err, service.ErrLoginDB), errors.Is(err, service.ErrTokenGenerate):
			httperr.Write(c, http.StatusInternalServerError, "Internal Server Error")
		default:
			httperr.Write(c, http.StatusInternalServerError, "Internal Server Error")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
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
// @Success 201 {string} string	"Successfully registered"
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
	c.JSON(http.StatusCreated, gin.H{"message": "Registration successful"})
}
