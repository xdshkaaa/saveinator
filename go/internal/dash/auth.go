package dash

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Telegram Login Widget auth. The widget signs its callback payload with
// HMAC-SHA256 over the bot token ("WebAppData" secret key), and the allowed
// operator is the single Telegram user id in DASH_ADMIN_IDS.
//
// Sessions are opaque random tokens kept in an in-memory map (single
// operator, single instance) and delivered as an HttpOnly cookie.

const (
	sessionCookie = "dash_session"
	sessionTTL    = 7 * 24 * time.Hour
)

type sessionManager struct {
	mu   sync.Mutex
	byID map[string]sessionEntry
}

type sessionEntry struct {
	userID    int64
	username  string
	firstName string
	expires   time.Time
}

func newSessionManager() *sessionManager {
	return &sessionManager{byID: map[string]sessionEntry{}}
}

func (m *sessionManager) issue(userID int64, username, firstName string) (token string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", err
	}
	token = base64.RawURLEncoding.EncodeToString(buf)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupLocked()
	m.byID[token] = sessionEntry{
		userID:    userID,
		username:  username,
		firstName: firstName,
		expires:   time.Now().Add(sessionTTL),
	}
	return token, nil
}

// lookup returns the session entry if the token is valid; expired sessions
// are removed on sight.
func (m *sessionManager) lookup(token string) (sessionEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.byID[token]
	if !ok {
		return sessionEntry{}, false
	}
	if time.Now().After(e.expires) {
		delete(m.byID, token)
		return sessionEntry{}, false
	}
	return e, true
}

func (m *sessionManager) revoke(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.byID, token)
}

func (m *sessionManager) cleanupLocked() {
	now := time.Now()
	for tok, e := range m.byID {
		if now.After(e.expires) {
			delete(m.byID, tok)
		}
	}
}

// telegramAuth validates the Login Widget callback fields (id, first_name,
// username, auth_date, hash) against the bot token and returns the claimed
// user. It follows the official algorithm:
// https://core.telegram.org/widgets/login#checking-authorization
func telegramAuth(token string, form map[string]string) (int64, string, string, error) {
	hash := form["hash"]
	if hash == "" {
		return 0, "", "", errors.New("missing hash")
	}
	authDate, err := strconv.ParseInt(form["auth_date"], 10, 64)
	if err != nil {
		return 0, "", "", errors.New("bad auth_date")
	}
	if time.Now().Unix()-authDate > 24*3600 {
		return 0, "", "", errors.New("stale auth_date")
	}

	var keys []string
	for k := range form {
		// Telegram подписывает только непустые поля: пустые значения в
		// проверочной строке ломают HMAC.
		if k != "hash" && form[k] != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(form[k])
	}

	secret := sha256.Sum256([]byte(token))
	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(b.String()))
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(hash)) {
		return 0, "", "", errors.New("bad signature")
	}

	userID, err := strconv.ParseInt(form["id"], 10, 64)
	if err != nil || userID <= 0 {
		return 0, "", "", errors.New("bad user id")
	}
	return userID, form["username"], form["first_name"], nil
}

// authUser extracts the session from the request cookie.
func (s *Server) authUser(r *http.Request) (sessionEntry, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return sessionEntry{}, false
	}
	return s.sessions.lookup(c.Value)
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	})
}

// requireAuth wraps a handler, answering 401 JSON for unauthenticated calls.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.authUser(r); !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	e, ok := s.authUser(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"authed": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authed": true,
		"user": map[string]any{
			"id":         e.userID,
			"username":   e.username,
			"first_name": e.firstName,
		},
	})
}

// handleAuthLogin validates the Telegram Login Widget callback and issues a
// session for the allowed operator. The form arrives as
// application/x-www-form-urlencoded (the widget's post_message → hidden form
// → redirect flow). Responding with the token in a JSON body lets the
// frontend finish the redirect without a visible intermediate page.
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad form"})
		return
	}
	form := make(map[string]string, len(r.PostForm))
	for k, vs := range r.PostForm {
		if len(vs) > 0 {
			form[k] = vs[0]
		}
	}

	userID, username, firstName, err := telegramAuth(s.telegramToken, form)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "telegram check failed"})
		return
	}
	if !s.isAdmin(userID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not allowed"})
		return
	}

	token, err := s.sessions.issue(userID, username, firstName)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session issue failed"})
		return
	}
	s.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{"authed": true})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.sessions.revoke(c.Value)
	}
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"authed": false})
}

func (s *Server) isAdmin(userID int64) bool {
	for _, id := range s.adminIDs {
		if id == userID {
			return true
		}
	}
	return false
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
