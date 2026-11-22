package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"typonamer/config"
	"typonamer/log"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

type LoginInfo struct {
	Username string `json:"username"`
	Password string `json:"password"`
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
		return c.SendStatus(fiber.StatusBadRequest)
	}
	claims := jwt.MapClaims{
		"name": cfg.AuthUsername,
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

func tokenExpiry(raw string) (time.Time, error) {
	cfg := config.GetConfig()
	if len(raw) == 0 || len(raw) > 4096 || cfg.JwtSecretKey == "" {
		return time.Time{}, errInvalidToken
	}
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) {
		return []byte(cfg.JwtSecretKey), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil || !token.Valid || claims["name"] != cfg.AuthUsername {
		return time.Time{}, errInvalidToken
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return time.Time{}, errInvalidToken
	}
	return exp.Time, nil
}
