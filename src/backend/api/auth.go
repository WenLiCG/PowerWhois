package api

import (
	"fmt"
	"time"

	"typonamer/config"
	"typonamer/log"

	jwtware "github.com/gofiber/contrib/jwt"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

const (
	// jwtDefaultExpireDays is the default number of days a JWT token is valid.
	// If the config value AuthExpireDays is not set, it uses the default value of jwtDefaultExpireDays.
	jwtDefaultExpireDays int = 1
)

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

	// Compare the username and password in the request body with the config values.
	if (login.Username != cfg.AuthUsername) || (login.Password != cfg.AuthPassword) {
		log.Warnf("Login failed. Username: %s, Password: %s", login.Username, login.Password)
		return c.SendStatus(fiber.StatusBadRequest) // Return a 400 status if the username or password is incorrect.
	}

	// Generate a JWT token with the username.
	token, err := generateToken(login.Username)
	if err != nil {
		log.Error("Generate token error: ", err)
		return c.SendStatus(fiber.StatusInternalServerError) // Return a 500 status if the token generation fails.
	}

	log.Infof("Login success with username: %s", login.Username)

	// Return the username and token in the response body.
	return c.JSON(fiber.Map{"username": cfg.AuthUsername, "token": token})
}

// LoginRequired returns a middleware that enforces JWT authentication.
//
// The middleware validates the JWT token in the Authorization header using the
// configured auth password as the signing key. Invalid or missing tokens result
// in a 401 Unauthorized response.
func LoginRequired() func(*fiber.Ctx) error {
	return jwtware.New(jwtware.Config{
		// SigningKey is the key used to sign the JWT token.
		// It is used to verify the token when it is received.
		SigningKey: jwtware.SigningKey{Key: []byte(config.GetConfig().JwtSecretKey)},
		// ErrorHandler is the handler that is called when the token is invalid, expired, or missing.
		// It returns a 401 status and a JSON object with status=error and a message of the error.
		ErrorHandler: jwtError,
	})
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
	token, err := jwt.Parse(tokenString, func(jwtToken *jwt.Token) (interface{}, error) {
		// Check the signing method of the JWT token.
		// Only HMAC signing method is supported.
		if _, ok := jwtToken.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected jwt signing method: %+v", jwtToken.Header["alg"])
		}

		// Return the signing key which is the JwtSecretKey config value.
		return []byte(config.GetConfig().JwtSecretKey), nil
	})

	if err != nil {
		// Log the error if the token parsing fails.
		log.Errorf("Failed to parse token: %s", err)
		return false, err
	}

	// Check if the token is valid.
	// If the token is invalid, expired, or missing, it will return false and an error.
	if _, ok := token.Claims.(jwt.MapClaims); !ok {
		log.Error("Parse token claims error: ", err)
		return false, fmt.Errorf("parse token claims error")
	}

	// Return the validation result.
	return token.Valid, nil
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
	claims := jwt.MapClaims{
		"name": username,
		"exp":  time.Now().Add(time.Hour * 24 * time.Duration(jwtExp)).Unix(),
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
