package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const sessionCookie = "lrp_session"

type Auth struct {
	bootstrapToken string
	mu             sync.Mutex
	sessions       map[string]time.Time
	now            func() time.Time
}

func NewAuth(bootstrapToken string) *Auth {
	return &Auth{bootstrapToken: bootstrapToken, sessions: make(map[string]time.Time), now: time.Now}
}

func (a *Auth) Enabled() bool { return a != nil && a.bootstrapToken != "" }

func (a *Auth) Wrap(next http.Handler) http.Handler {
	if !a.Enabled() {
		return next
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost && request.URL.Path == "/api/v1/auth/exchange" {
			a.exchange(writer, request)
			return
		}
		if a.validBearer(request.Header.Get("Authorization")) {
			next.ServeHTTP(writer, request)
			return
		}
		cookie, err := request.Cookie(sessionCookie)
		if err != nil || !a.validSession(cookie.Value) {
			writeProblem(writer, http.StatusUnauthorized, "authentication_required", "run `lrp dashboard` to open an authenticated session")
			return
		}
		if request.Method != http.MethodGet && request.Method != http.MethodHead && !sameLoopbackOrigin(request) {
			writeProblem(writer, http.StatusForbidden, "invalid_origin", "state-changing browser requests require a same-origin dashboard request")
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (a *Auth) exchange(writer http.ResponseWriter, request *http.Request) {
	if !sameLoopbackOrigin(request) {
		writeProblem(writer, http.StatusForbidden, "invalid_origin", "login requires the loopback dashboard origin")
		return
	}
	var input struct {
		Token string `json:"token"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 4<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || !secureEqual(input.Token, a.bootstrapToken) {
		writeProblem(writer, http.StatusUnauthorized, "invalid_token", "bootstrap token is invalid")
		return
	}
	value := a.newSession()
	http.SetCookie(writer, &http.Cookie{
		Name: sessionCookie, Value: value, Path: "/api", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, MaxAge: int((12 * time.Hour).Seconds()),
	})
	writer.WriteHeader(http.StatusNoContent)
}

func (a *Auth) newSession() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	value := base64.RawURLEncoding.EncodeToString(bytes)
	a.mu.Lock()
	a.sessions[value] = a.now().Add(12 * time.Hour)
	for token, expiry := range a.sessions {
		if expiry.Before(a.now()) {
			delete(a.sessions, token)
		}
	}
	a.mu.Unlock()
	return value
}

func (a *Auth) validSession(value string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	expiry, ok := a.sessions[value]
	if !ok || !expiry.After(a.now()) {
		delete(a.sessions, value)
		return false
	}
	return true
}

func (a *Auth) validBearer(value string) bool {
	token, ok := strings.CutPrefix(value, "Bearer ")
	return ok && secureEqual(token, a.bootstrapToken)
}

func secureEqual(left, right string) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func sameLoopbackOrigin(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == "" {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "http" || parsed.Host != request.Host {
		return false
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
