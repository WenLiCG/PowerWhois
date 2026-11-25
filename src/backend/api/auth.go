package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"typonamer/config"
	"typonamer/log"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

const (
	// jwtDefaultExpireDays is the default number of days a JWT token is valid.
	// If the config value AuthExpireDays is not set, it uses the default value of jwtDefaultExpireDays.
	jwtDefaultExpireDays int = 1
	authGenerationKey        = "powerwhois:auth:generation"
)

var authGeneration atomic.Int64
var authStore atomic.Pointer[redis.Client]
var logoutMu sync.Mutex

// InitAuth restores the administrator generation before the server accepts requests.
func InitAuth(ctx context.Context, r *redis.Client) error {
	generation, err := r.Get(ctx, authGenerationKey).Int64()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	if generation < 0 {
		return errors.New("invalid administrator session generation")
	}
	authStore.Store(r)
	authGeneration.Store(generation)
	return nil
}

type adminClaims struct {
	Name       string `json:"name"`
	Generation int64  `json:"gen"`
	jwt.RegisteredClaims
}

// LoginInfo contains the credentials for user authentication.
type LoginInfo struct {
	// Username is the login username
	Username string `json:"username"`
	// Password is the login password
	Password string `json:"password"`
}

// Login handles user authentication and JWT token generation.
//
// It validates the provided username and password against configured credentials.
// On successful authentication, it generates and returns a JWT token.
//
// Returns:
//   - 200: Successful login with username and token
//   - 400: Invalid credentials or malformed request
//   - 500: Internal server error during token generation
func Login(c *fiber.Ctx) error {

	// Parse the request body and get the login information.
	var login LoginInfo
	if err := c.BodyParser(&login); err != nil {
		log.Error("Parse login info error: ", err)
		return c.SendStatus(fiber.StatusBadRequest) // Return a 400 status if the parse fails.
	}

	// Get the config information.
	cfg := config.GetConfig()

	// Compare fixed-size digests in constant time to avoid leaking credential length or prefix.
	userDigest := sha256.Sum256([]byte(login.Username))
	expectedUser := sha256.Sum256([]byte(cfg.AuthUsername))
	passDigest := sha256.Sum256([]byte(login.Password))
	expectedPass := sha256.Sum256([]byte(cfg.AuthPassword))
	userMatch := subtle.ConstantTimeCompare(userDigest[:], expectedUser[:]) == 1
	passMatch := subtle.ConstantTimeCompare(passDigest[:], expectedPass[:]) == 1
	if !userMatch || !passMatch {
		log.Warn("Login failed")
		return c.SendStatus(fiber.StatusBadRequest) // Return a 400 status if the username or password is incorrect.
	}

	// Generate a JWT token with the username.
	token, err := generateToken(login.Username)
	if err != nil {
		log.Error("Generate token error: ", err)
		return c.SendStatus(fiber.StatusInternalServerError) // Return a 500 status if the token generation fails.
	}

	// Return the username and token in the response body.
	c.Set("Cache-Control", "no-store")
	return c.JSON(fiber.Map{"username": cfg.AuthUsername, "token": token})
}

// Logout revokes all administrator sessions for this installation.
func Logout(c *fiber.Ctx) error {
	logoutMu.Lock()
	defer logoutMu.Unlock()
	token, ok := strings.CutPrefix(c.Get("Authorization"), "Bearer ")
	if !ok || !validAdminToken(token) {
		return jwtError(c, errors.New("invalid or expired credential"))
	}
	store := authStore.Load()
	if store == nil {
		return fiber.ErrServiceUnavailable
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 3*time.Second)
	defer cancel()
	next, err := store.Incr(ctx, authGenerationKey).Result()
	if err != nil {
		log.Error("Cannot revoke administrator sessions")
		return fiber.ErrServiceUnavailable
	}
	authGeneration.Store(next)
	return c.SendStatus(fiber.StatusNoContent)
}

func validAdminToken(token string) bool {
	valid, _ := ValidateToken(token)
	return valid
}

// LoginRequired returns a middleware that enforces JWT authentication.
//
// The middleware validates the JWT token in the Authorization header using the
// configured JWT secret as the signing key. Invalid or missing tokens result
// in a 401 Unauthorized response.
func LoginRequired() func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		header := c.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			return jwtError(c, fmt.Errorf("authentication required"))
		}
		if !validAdminToken(strings.TrimPrefix(header, "Bearer ")) {
			return jwtError(c, fmt.Errorf("invalid or expired credential"))
		}
		return c.Next()
	}
}

// ValidateToken validates a JWT token string.
//
// Parameters:
//   - tokenString: The JWT token to validate
//
// Returns:
//   - bool: True if token is valid, false otherwise
//   - error: Error details if validation fails, nil on success
func ValidateToken(tokenString string) (bool, error) {
	if len(tokenString) == 0 || len(tokenString) > 4096 {
		return false, errors.New("invalid administrator token")
	}
	cfg := config.GetConfig()
	claims := &adminClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(*jwt.Token) (any, error) {
		return []byte(cfg.JwtSecretKey), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return false, err
	}
	// Tokens issued before session generations existed default to generation zero.
	return token.Valid && claims.Name == cfg.AuthUsername && claims.Generation == authGeneration.Load(), nil
}

// jwtError handles JWT authentication errors by returning a 401 response.
//
// Parameters:
//   - c: The Fiber context
//   - err: The JWT validation error
//
// Returns:
//   - error: The error from sending the response
func jwtError(c *fiber.Ctx, err error) error {
	return c.Status(401).JSON(fiber.Map{"status": "error", "message": err.Error()})
}

// generateToken creates a new JWT token for the given username.
//
// The token includes the username claim and expires after the configured number
// of days (defaults to jwtDefaultExpireDays if not configured).
//
// Parameters:
//   - username: The username to include in the token claims
//
// Returns:
//   - string: The signed JWT token string
//   - error: Error if token generation fails
func generateToken(username string) (string, error) {
	cfg := config.GetConfig()

	// Get the number of days the token is valid for.
	// If the AuthExpireDays config value is not set, use the default value of jwtDefaultExpireDays.
	var jwtExp int
	if cfg.AuthExpireDays > 0 {
		jwtExp = cfg.AuthExpireDays
	} else {
		jwtExp = jwtDefaultExpireDays
	}

	// Create the Claims
	claims := adminClaims{
		Name:       username,
		Generation: authGeneration.Load(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 24 * time.Duration(jwtExp))),
		},
	}

	// Create token
	tokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Generate encoded token and send it as response.
	token, err := tokenObj.SignedString([]byte(cfg.JwtSecretKey))
	if err != nil {
		// Log the error if the token generation fails.
		log.Error("Generate token error: ", err)
		return "", err
	}

	return token, nil
}
