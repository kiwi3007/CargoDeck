package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ---- Brute-force rate limiter for /auth/verify ----

const (
	authRateMaxAttempts = 5
	authRateWindow      = 5 * time.Minute
)

var (
	authAttempts   = map[string][]time.Time{}
	authAttemptsMu sync.Mutex
)

// authRateLimitOK returns true if the IP is within the allowed attempt rate.
// Uses a sliding window: max 5 attempts per 5 minutes per IP.
func authRateLimitOK(ip string) bool {
	authAttemptsMu.Lock()
	defer authAttemptsMu.Unlock()
	now := time.Now()
	cutoff := now.Add(-authRateWindow)
	recent := authAttempts[ip][:0]
	for _, t := range authAttempts[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= authRateMaxAttempts {
		authAttempts[ip] = recent
		return false
	}
	authAttempts[ip] = append(recent, now)
	return true
}

// remoteIP extracts just the IP (no port) from r.RemoteAddr.
func remoteIP(r *http.Request) string {
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[:i]
	}
	return addr
}

// maskSentinel is returned by masked GET endpoints to indicate "value set but not shown".
// Save handlers detect this value and leave the existing credential unchanged.
const maskSentinel = "••••••••"

// ---- OTP store (single-slot, 15-min TTL) ----

type otpEntry struct {
	code    string
	expires time.Time
}

var otpMu sync.Mutex
var activeOTP *otpEntry // nil = no active OTP

// ---- UI nonce store (60s TTL, single-use) ----

var uiNonces sync.Map // string → time.Time (expires)

// ---- UI session store (24h TTL) ----

var uiSessions sync.Map // string → time.Time (expires)

// ---- Context key for OTP-consumed requests ----

type otpConsumedKey struct{}

// ---- POST /api/v3/auth/otp/generate ----

// GenerateOTP creates a 6-char OTP with a 15-minute TTL.
// Requires a valid UI session (protected by uiAuthMiddleware).
func (h *Handler) GenerateOTP(w http.ResponseWriter, r *http.Request) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		jsonErr(w, 500, "rng error")
		return
	}
	code := make([]byte, 6)
	for i, v := range raw {
		code[i] = charset[int(v)%len(charset)]
	}
	exp := time.Now().Add(15 * time.Minute)
	otpMu.Lock()
	activeOTP = &otpEntry{code: string(code), expires: exp}
	otpMu.Unlock()
	jsonOK(w, map[string]any{
		"otp":       string(code),
		"expiresAt": exp.UTC().Format(time.RFC3339),
	})
}

// ---- GET /api/v3/auth/challenge ----

// GetChallenge issues a single-use 32-byte hex nonce for CHAP-SHA256 login.
func (h *Handler) GetChallenge(w http.ResponseWriter, r *http.Request) {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		jsonErr(w, 500, "rng error")
		return
	}
	nonceHex := hex.EncodeToString(nonce)
	uiNonces.Store(nonceHex, time.Now().Add(60*time.Second))
	jsonOK(w, map[string]any{"nonce": nonceHex, "expiresIn": 60})
}

// ---- GET /api/v3/auth/required ----

func (h *Handler) GetAuthRequired(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg.LoadServer()
	jsonOK(w, map[string]bool{"required": cfg.UIPassword != ""})
}

// ---- POST /api/v3/auth/verify ----

// VerifyAuth performs CHAP-SHA256: the client sends the nonce it received from
// /auth/challenge along with HMAC-SHA256(password, nonce). On success a 24h
// session token is returned; the raw password is never transmitted.
func (h *Handler) VerifyAuth(w http.ResponseWriter, r *http.Request) {
	if !authRateLimitOK(remoteIP(r)) {
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}
	var body struct {
		Nonce    string `json:"nonce"`
		Response string `json:"response"`
	}
	if err := decodeBody(r, &body); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if body.Nonce == "" || body.Response == "" {
		jsonErr(w, 400, "nonce and response are required")
		return
	}

	// Validate and burn nonce (single-use)
	val, ok := uiNonces.LoadAndDelete(body.Nonce)
	if !ok {
		jsonErr(w, 401, "invalid or expired nonce")
		return
	}
	exp, ok2 := val.(time.Time)
	if !ok2 || time.Now().After(exp) {
		jsonErr(w, 401, "nonce expired")
		return
	}

	cfg := h.cfg.LoadServer()
	if cfg.UIPassword == "" {
		jsonErr(w, 400, "no password set")
		return
	}

	// Compute expected HMAC-SHA256(password, nonce)
	mac := hmac.New(sha256.New, []byte(cfg.UIPassword))
	mac.Write([]byte(body.Nonce))
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(body.Response), []byte(expected)) {
		jsonErr(w, 401, "invalid credentials")
		return
	}

	// Issue 24h session token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		jsonErr(w, 500, "failed to generate session token")
		return
	}
	token := hex.EncodeToString(tokenBytes)
	uiSessions.Store(token, time.Now().Add(24*time.Hour))

	jsonOK(w, map[string]string{"token": token})
}

// isValidUISession returns true if the token exists in uiSessions and is not expired.
func isValidUISession(token string) bool {
	if val, ok := uiSessions.Load(token); ok {
		if exp, ok := val.(time.Time); ok && time.Now().Before(exp) {
			return true
		}
		uiSessions.Delete(token)
	}
	return false
}

// uiAuthMiddleware enforces UI password auth when UIPassword is set.
// Accepts:
//   - A valid UI session token in "Authorization: Bearer <token>" or "?auth=<token>"
//   - A valid agent session token (agents calling protected endpoints)
//   - A valid OTP in "?otp=<code>" (single-use, burns on consumption)
//
// Exempt paths: /health, specific /api/v3/auth/* endpoints, /api/v3/agent/register
func (h *Handler) uiAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := h.cfg.LoadServer()
		password := cfg.UIPassword

		// No password set — pass through (LAN mode)
		if password == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Specific exempt paths (auth bootstrap + non-sensitive public endpoints)
		switch r.URL.Path {
		case "/health",
			"/api/v3/auth/required",
			"/api/v3/auth/verify",
			"/api/v3/auth/challenge",
			"/api/v3/auth/agent-challenge",
			"/api/v3/agent/register",
			"/api/v3/agent/binary",  // binary download is not sensitive
			"/api/v3/agent/version": // version check used by agents on startup
			next.ServeHTTP(w, r)
			return
		}

		// Extract token from header or query param
		token := ""
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		}
		if token == "" {
			token = r.URL.Query().Get("auth")
		}

		// Check UI session token
		if token != "" && isValidUISession(token) {
			next.ServeHTTP(w, r)
			return
		}

		// Check agent session token (agents call through uiAuthMiddleware too)
		if token != "" && isValidAgentSessionToken(token) {
			next.ServeHTTP(w, r)
			return
		}

		// Check OTP in query param (single-use, burned on consumption)
		if otp := r.URL.Query().Get("otp"); otp != "" {
			otpMu.Lock()
			entry := activeOTP
			valid := entry != nil && entry.code == otp && time.Now().Before(entry.expires)
			if valid {
				activeOTP = nil // burn on use
			}
			otpMu.Unlock()
			if valid {
				ctx := context.WithValue(r.Context(), otpConsumedKey{}, true)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		w.Header().Set("WWW-Authenticate", `Bearer realm="Playerr"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}

// securityHeaders adds HTTP security headers to all responses.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self' ws: wss:")
		next.ServeHTTP(w, r)
	})
}
