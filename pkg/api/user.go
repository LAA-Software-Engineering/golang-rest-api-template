package api

import (
	"errors"
	"net/http"

	"golang-rest-api-template/pkg/models"
	"golang-rest-api-template/pkg/repository"
	"golang-rest-api-template/pkg/service"

	"github.com/gin-gonic/gin"
)

// UserRepository is the HTTP surface for auth routes (Gin handlers).
type UserRepository interface {
	LoginHandler(c *gin.Context)
	RegisterHandler(c *gin.Context)
}

type userRepository struct {
	svc *service.UserService
}

// NewUserRepository wires persistence into user HTTP handlers.
func NewUserRepository(store repository.UserPersistence) *userRepository {
	return &userRepository{svc: service.NewUserService(store)}
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
func (r *userRepository) LoginHandler(c *gin.Context) {
	var incoming models.LoginUser
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	token, err := r.svc.Login(c.Request.Context(), incoming.Username, incoming.Password)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidLogin):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
		case errors.Is(err, service.ErrLoginDB), errors.Is(err, service.ErrTokenGenerate):
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
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
func (r *userRepository) RegisterHandler(c *gin.Context) {
	var user models.LoginUser
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := r.svc.Register(c.Request.Context(), user.Username, user.Password); err != nil {
		switch {
		case errors.Is(err, service.ErrRegisterConflict):
			c.JSON(http.StatusConflict, gin.H{"error": "username already taken"})
		case errors.Is(err, service.ErrRegisterHash), errors.Is(err, service.ErrRegisterSave):
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not save user"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not save user"})
		}
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "Registration successful"})
}
