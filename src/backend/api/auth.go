package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"typonamer/config"
	"typonamer/log"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

type LoginInfo struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// authGenerationKey stores the administrator session generation. Logout
// increments it, which revokes every token issued for an earlier generation.
const authGenerationKey = "powerwhois:auth:generation"

var authGeneration atomic.Int64
var authStore atomic.Pointer[redis.Client]

// InitAuth loads the session generation so a logout survives restarts.
// A nil client keeps the generation in memory only.
func InitAuth(ctx context.Context, client *redis.Client) error {
	if client == nil {
		return nil
	}
	authStore.Store(client)
	value, err := client.Get(ctx, authGenerationKey).Int64()
	if errors.Is(err, redis.Nil) {
		value, err = 0, nil
	}
	if err != nil {
		return err
	}
	authGeneration.Store(value)
	return nil
}

func Login(c *fiber.Ctx) error {
	var login LoginInfo
	if err := readJSON(c, &login, 8<<10); err != nil {
		return err
	}
	cfg := config.GetConfig()
	u, p := sha256.Sum256([]byte(login.Username)), sha256.Sum256([]byte(login.Password))
	expectedUser, expectedPassword := sha256.Sum256([]byte(cfg.AuthUsername)), sha256.Sum256([]byte(cfg.AuthPassword))
	if cfg.AuthPassword == "" || subtle.ConstantTimeCompare(u[:], expectedUser[:])&subtle.ConstantTimeCompare(p[:], expectedPassword[:]) == 0 {
		log.Warn("Login failed")
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	claims := jwt.MapClaims{
		"name": cfg.AuthUsername,
		"gen":  authGeneration.Load(),
		"iat":  time.Now().Unix(),
		"exp":  time.Now().Add(24 * time.Hour * time.Duration(cfg.AuthExpireDays)).Unix(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.JwtSecretKey))
	if err != nil {
		log.Error("Could not issue administrator token")
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	c.Set("Cache-Control", "no-store")
	return c.JSON(fiber.Map{"username": cfg.AuthUsername, "token": token})
}

// Logout revokes all administrator tokens, including those in other browsers.
func Logout(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.UserContext(), 3*time.Second)
	defer cancel()
	if client := authStore.Load(); client != nil {
		next, err := client.Incr(ctx, authGenerationKey).Result()
		if err != nil {
			log.Error("Could not revoke administrator sessions: ", err)
			return fiber.ErrServiceUnavailable
		}
		authGeneration.Store(next)
	} else {
		authGeneration.Add(1)
	}
	log.Info("Administrator sessions revoked")
	return c.SendStatus(fiber.StatusNoContent)
}

func LoginRequired(c *fiber.Ctx) error {
	scheme, token, _ := strings.Cut(c.Get("Authorization"), " ")
	if !strings.EqualFold(scheme, "Bearer") {
		return fiber.ErrUnauthorized
	}
	if _, err := tokenExpiry(token); err != nil {
		return fiber.ErrUnauthorized
	}
	return c.Next()
}

var errInvalidToken = errors.New("invalid administrator session")

type adminToken struct {
	expiry     time.Time
	generation int64
}

func tokenExpiry(raw string) (time.Time, error) {
	token, err := parseAdminToken(raw)
	return token.expiry, err
}

func parseAdminToken(raw string) (adminToken, error) {
	cfg := config.GetConfig()
	if len(raw) == 0 || len(raw) > 4096 || cfg.JwtSecretKey == "" {
		return adminToken{}, errInvalidToken
	}
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) {
		return []byte(cfg.JwtSecretKey), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil || !token.Valid || claims["name"] != cfg.AuthUsername {
		return adminToken{}, errInvalidToken
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return adminToken{}, errInvalidToken
	}
	// Tokens issued before this release carry no generation and count as 0.
	var generation int64
	if value, ok := claims["gen"]; ok {
		number, ok := value.(float64)
		if !ok || number != math.Trunc(number) || number < 0 {
			return adminToken{}, errInvalidToken
		}
		generation = int64(number)
	}
	if generation != authGeneration.Load() {
		return adminToken{}, errInvalidToken
	}
	return adminToken{expiry: exp.Time, generation: generation}, nil
}

// apiKeyHashes holds SHA-256 digests of POWERWHOIS_API_KEYS. ApiRoute sets it
// before the server accepts requests.
var apiKeyHashes atomic.Pointer[[][sha256.Size]byte]

func setAPIKeys(value string) {
	keys := loadAPIKeys(value)
	apiKeyHashes.Store(&keys)
}

func configuredAPIKeys() [][sha256.Size]byte {
	if keys := apiKeyHashes.Load(); keys != nil {
		return *keys
	}
	return nil
}

func loadAPIKeys(value string) [][sha256.Size]byte {
	var keys [][sha256.Size]byte
	for _, key := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' }) {
		if len(key) >= 24 {
			keys = append(keys, sha256.Sum256([]byte(key)))
		} else {
			log.Warn("Ignoring a POWERWHOIS_API_KEYS entry shorter than 24 characters")
		}
	}
	return keys
}

func requestAPIKey(c *fiber.Ctx) string {
	if key := c.Get("X-API-Key"); key != "" {
		return key
	}
	scheme, key, _ := strings.Cut(c.Get("Authorization"), " ")
	if strings.EqualFold(scheme, "Bearer") {
		return key
	}
	return ""
}

func validAPIKey(key string) bool {
	if key == "" || len(key) > 512 {
		return false
	}
	sum := sha256.Sum256([]byte(key))
	match := 0
	for _, expected := range configuredAPIKeys() {
		match |= subtle.ConstantTimeCompare(sum[:], expected[:])
	}
	return match == 1
}

// APIKeyRequired protects the integration API. It answers 404 when no keys
// are configured so the endpoint is invisible on installations that do not use it.
func APIKeyRequired(c *fiber.Ctx) error {
	if len(configuredAPIKeys()) == 0 {
		return fiber.ErrNotFound
	}
	if !validAPIKey(requestAPIKey(c)) {
		return fiber.ErrUnauthorized
	}
	return c.Next()
}
