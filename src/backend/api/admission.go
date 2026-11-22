package api

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Administrator WebSocket admission.
//
// Anonymous visitors share the public connection pool, so they can fill it
// before an administrator's adminAuth message is ever read. Administrators
// therefore authenticate before the upgrade: an authenticated HTTP request
// (POST /api/admin/ws-ticket) returns a one-time ticket, and the browser
// connects to /app/ws?ticket=... . A valid ticket admits the connection into a
// reserved pool that anonymous clients cannot reach, with administrator access
// already granted. HTTP requests are not limited by the WebSocket pools, so the
// ticket endpoint stays reachable while the public pool is full.
//
// Tickets expire after ticketTTL, are deleted on first use, and are bound to
// the session generation, so a logout also invalidates unused tickets. The
// ticket travels in the URL and may appear in proxy access logs; it is useless
// once redeemed or expired.

const (
	ticketTTL = 30 * time.Second
	// maxTickets bounds memory; only authenticated administrators can issue tickets.
	maxTickets = 256
	// adminConnections is the reserved pool for ticket-admitted connections.
	adminConnections = 8
	// publicConnections is the pool shared by connections without a ticket.
	publicConnections = 64
)

type wsTicket struct {
	expires time.Time
	token   adminToken
}

var ticketsMu sync.Mutex
var tickets = map[string]wsTicket{}

func bearerToken(c *fiber.Ctx) string {
	scheme, token, _ := strings.Cut(c.Get("Authorization"), " ")
	if !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return token
}

// IssueWSTicket returns a one-time ticket for an administrator WebSocket connection.
func IssueWSTicket(c *fiber.Ctx) error {
	token, err := parseAdminToken(bearerToken(c))
	if err != nil {
		return fiber.ErrUnauthorized
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return fiber.ErrInternalServerError
	}
	ticket := base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now()
	ticketsMu.Lock()
	for key, value := range tickets {
		if !now.Before(value.expires) {
			delete(tickets, key)
		}
	}
	if len(tickets) >= maxTickets {
		ticketsMu.Unlock()
		return fiber.ErrServiceUnavailable
	}
	tickets[ticket] = wsTicket{expires: now.Add(ticketTTL), token: token}
	ticketsMu.Unlock()
	c.Set("Cache-Control", "no-store")
	return c.JSON(fiber.Map{"ticket": ticket, "expiresIn": int(ticketTTL / time.Second)})
}

// redeemTicket consumes a ticket and returns the administrator session it
// grants. It fails for unknown, expired or revoked tickets.
func redeemTicket(ticket string) (adminToken, bool) {
	if ticket == "" || len(ticket) > 128 {
		return adminToken{}, false
	}
	ticketsMu.Lock()
	value, ok := tickets[ticket]
	delete(tickets, ticket)
	ticketsMu.Unlock()
	now := time.Now()
	if !ok || !now.Before(value.expires) || !now.Before(value.token.expiry) ||
		value.token.generation != authGeneration.Load() {
		return adminToken{}, false
	}
	return value.token, true
}

// dropTickets discards every outstanding ticket. Logout calls it so revoked
// sessions cannot use tickets issued earlier.
func dropTickets() {
	ticketsMu.Lock()
	clear(tickets)
	ticketsMu.Unlock()
}
