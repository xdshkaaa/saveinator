package dash

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"
)

const testBotToken = "123456:TEST-BOT-TOKEN-abcdef"

// signTelegramForm builds a valid Login Widget callback for the given user.
func signTelegramForm(t *testing.T, userID int64, username, firstName string, authDate int64, extra map[string]string) url.Values {
	t.Helper()
	form := url.Values{}
	form.Set("id", fmt.Sprint(userID))
	form.Set("auth_date", fmt.Sprint(authDate))
	if username != "" {
		form.Set("username", username)
	}
	if firstName != "" {
		form.Set("first_name", firstName)
	}
	for k, v := range extra {
		form.Set(k, v)
	}

	var keys []string
	for k := range form {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(form.Get(k))
	}

	secret := sha256.Sum256([]byte(testBotToken))
	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(b.String()))
	form.Set("hash", hex.EncodeToString(mac.Sum(nil)))
	return form
}

func newTestServer() *Server {
	s := &Server{
		sessions:      newSessionManager(),
		telegramToken: testBotToken,
		adminIDs:      []int64{339193247},
	}
	return s
}

func TestTelegramAuthValid(t *testing.T) {
	form := signTelegramForm(t, 339193247, "xdshka", "Xdshka", time.Now().Unix(), nil)
	m := make(map[string]string, len(form))
	for k, vs := range form {
		m[k] = vs[0]
	}
	id, user, first, err := telegramAuth(testBotToken, m)
	if err != nil {
		t.Fatalf("valid auth rejected: %v", err)
	}
	if id != 339193247 || user != "xdshka" || first != "Xdshka" {
		t.Fatalf("wrong identity: %d %q %q", id, user, first)
	}
}

// TestTelegramAuthSparseFields: Telegram подписывает только те поля, которые
// реально есть у пользователя. Пустые username/last_name/photo_url в форме
// не должны участвовать в проверке (и не должны её ломать).
func TestTelegramAuthSparseFields(t *testing.T) {
	form := signTelegramForm(t, 339193247, "", "Xdshka", time.Now().Unix(), nil)
	// Клиент может прислать пустые поля — сервер обязан их проигнорировать.
	form.Set("username", "")
	form.Set("last_name", "")
	form.Set("photo_url", "")
	m := make(map[string]string, len(form))
	for k, vs := range form {
		m[k] = vs[0]
	}
	id, user, first, err := telegramAuth(testBotToken, m)
	if err != nil {
		t.Fatalf("sparse auth rejected: %v", err)
	}
	if id != 339193247 || user != "" || first != "Xdshka" {
		t.Fatalf("wrong identity: %d %q %q", id, user, first)
	}
}

func TestTelegramAuthTampered(t *testing.T) {
	form := signTelegramForm(t, 339193247, "xdshka", "Xdshka", time.Now().Unix(), nil)
	form.Set("id", "12345") // tamper after signing
	m := make(map[string]string, len(form))
	for k, vs := range form {
		m[k] = vs[0]
	}
	if _, _, _, err := telegramAuth(testBotToken, m); err == nil {
		t.Fatal("tampered auth accepted")
	}
}

func TestTelegramAuthStale(t *testing.T) {
	form := signTelegramForm(t, 339193247, "xdshka", "Xdshka", time.Now().Unix()-48*3600, nil)
	m := make(map[string]string, len(form))
	for k, vs := range form {
		m[k] = vs[0]
	}
	if _, _, _, err := telegramAuth(testBotToken, m); err == nil {
		t.Fatal("stale auth accepted")
	}
}

func TestLoginFlow(t *testing.T) {
	s := newTestServer()
	r := s.Router()

	// data endpoints require auth
	req := httptest.NewRequest(http.MethodGet, "/api/overview", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated overview: got %d, want 401", rec.Code)
	}

	// wrong user is forbidden
	form := signTelegramForm(t, 42, "stranger", "Stranger", time.Now().Unix(), nil)
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin login: got %d, want 403", rec.Code)
	}

	// valid admin login issues a session cookie
	form = signTelegramForm(t, 339193247, "xdshka", "Xdshka", time.Now().Unix(), nil)
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin login: got %d, want 200", rec.Code)
	}
	cookies := rec.Result().Cookies()
	var session *http.Cookie
	for _, c := range cookies {
		if c.Name == sessionCookie {
			session = c
		}
	}
	if session == nil || session.Value == "" {
		t.Fatal("no session cookie issued")
	}

	// session cookie unlocks the API (overview will fail on DB, but not on auth)
	req = httptest.NewRequest(http.MethodGet, "/api/overview", nil)
	req.AddCookie(session)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatal("authenticated overview still 401")
	}

	// status reports the session
	req = httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	req.AddCookie(session)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"authed":true`) {
		t.Fatalf("auth status: %d %s", rec.Code, rec.Body.String())
	}

	// logout kills the session
	req = httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(session)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	req = httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	req.AddCookie(session)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `"authed":false`) {
		t.Fatalf("status after logout: %s", rec.Body.String())
	}
}
