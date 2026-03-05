package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
)

func TestNewHandler_Disabled(t *testing.T) {
	h := NewHandler(Config{})
	if h.enabled {
		t.Error("expected disabled when no ClientID/TenantID")
	}
	if h.IsEnabled() {
		t.Error("IsEnabled() should return false")
	}
}

func TestNewHandler_DisabledMissingTenantID(t *testing.T) {
	h := NewHandler(Config{ClientID: "id"})
	if h.enabled {
		t.Error("expected disabled when TenantID is missing")
	}
}

func TestNewHandler_Enabled(t *testing.T) {
	h := NewHandler(Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-secret",
		TenantID:     "test-tenant",
		RedirectURL:  "http://localhost:8080/auth/callback",
		SessionKey:   []byte("test-session-key-32-bytes-long!!"),
	})
	if !h.enabled {
		t.Error("expected enabled")
	}
	if !h.IsEnabled() {
		t.Error("IsEnabled() should return true")
	}
	if h.tenantID != "test-tenant" {
		t.Errorf("tenantID = %q", h.tenantID)
	}
}

func TestNewHandler_DefaultRedirectURL(t *testing.T) {
	h := NewHandler(Config{
		ClientID:   "test-client-id",
		TenantID:   "test-tenant",
		SessionKey: []byte("test-session-key-32-bytes-long!!"),
	})
	if h.oauthConfig.RedirectURL != "http://localhost:8080/auth/callback" {
		t.Errorf("RedirectURL = %q, want default", h.oauthConfig.RedirectURL)
	}
}

func TestNewHandler_GeneratesSessionKey(t *testing.T) {
	h := NewHandler(Config{
		ClientID: "test-client-id",
		TenantID: "test-tenant",
	})
	if len(h.sessionKey) == 0 {
		t.Error("expected auto-generated session key")
	}
}

func TestRequireAuth_Disabled(t *testing.T) {
	h := NewHandler(Config{})

	called := false
	handler := h.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if !called {
		t.Error("handler should be called directly when auth is disabled")
	}
}

func TestRequireAuth_NoSession(t *testing.T) {
	h := NewHandler(Config{
		ClientID:   "test-client-id",
		TenantID:   "test-tenant",
		SessionKey: []byte("test-session-key-32-bytes-long!!"),
	})

	handler := h.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called when no session")
	})

	req := httptest.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusFound)
	}
	location := w.Header().Get("Location")
	if location != "/auth/login" {
		t.Errorf("redirect location = %q, want /auth/login", location)
	}
}

func TestRequireAuth_ValidSession(t *testing.T) {
	sessionKey := []byte("test-session-key-32-bytes-long!!")
	h := NewHandler(Config{
		ClientID:   "test-client-id",
		TenantID:   "test-tenant",
		SessionKey: sessionKey,
	})

	// Create a valid session token
	claims := jwt.MapClaims{
		"name":  "Test User",
		"email": "test@example.com",
		"exp":   time.Now().Add(1 * time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString(sessionKey)

	var capturedUser *UserInfo
	handler := h.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		capturedUser = GetUserFromContext(r)
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: tokenString})
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if capturedUser == nil {
		t.Fatal("user should be in context")
	}
	if capturedUser.Name != "Test User" {
		t.Errorf("user name = %q", capturedUser.Name)
	}
	if capturedUser.Email != "test@example.com" {
		t.Errorf("user email = %q", capturedUser.Email)
	}
}

func TestRequireAuth_ExpiredSession(t *testing.T) {
	sessionKey := []byte("test-session-key-32-bytes-long!!")
	h := NewHandler(Config{
		ClientID:   "test-client-id",
		TenantID:   "test-tenant",
		SessionKey: sessionKey,
	})

	claims := jwt.MapClaims{
		"name":  "Test User",
		"email": "test@example.com",
		"exp":   time.Now().Add(-1 * time.Hour).Unix(),
		"iat":   time.Now().Add(-2 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString(sessionKey)

	handler := h.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with expired session")
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: tokenString})
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want redirect", w.Code)
	}
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	h := NewHandler(Config{
		ClientID:   "test-client-id",
		TenantID:   "test-tenant",
		SessionKey: []byte("test-session-key-32-bytes-long!!"),
	})

	handler := h.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with invalid token")
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "invalid-token"})
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want redirect", w.Code)
	}
}

func TestLogin_Disabled(t *testing.T) {
	h := NewHandler(Config{})

	req := httptest.NewRequest("GET", "/auth/login", nil)
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestLogin_Enabled(t *testing.T) {
	h := NewHandler(Config{
		ClientID:   "test-client-id",
		TenantID:   "test-tenant",
		SessionKey: []byte("test-session-key-32-bytes-long!!"),
	})

	req := httptest.NewRequest("GET", "/auth/login", nil)
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d (redirect)", w.Code, http.StatusFound)
	}

	location := w.Header().Get("Location")
	if location == "" {
		t.Error("expected redirect location")
	}

	// Should have oauth_state cookie
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "oauth_state" {
			found = true
			if c.Value == "" {
				t.Error("oauth_state cookie should have a value")
			}
		}
	}
	if !found {
		t.Error("expected oauth_state cookie")
	}
}

func TestCallback_Disabled(t *testing.T) {
	h := NewHandler(Config{})

	req := httptest.NewRequest("GET", "/auth/callback", nil)
	w := httptest.NewRecorder()
	h.Callback(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestCallback_InvalidState(t *testing.T) {
	h := NewHandler(Config{
		ClientID:   "test-client-id",
		TenantID:   "test-tenant",
		SessionKey: []byte("test-session-key-32-bytes-long!!"),
	})

	req := httptest.NewRequest("GET", "/auth/callback?state=wrong", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "expected"})
	w := httptest.NewRecorder()
	h.Callback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCallback_NoStateCookie(t *testing.T) {
	h := NewHandler(Config{
		ClientID:   "test-client-id",
		TenantID:   "test-tenant",
		SessionKey: []byte("test-session-key-32-bytes-long!!"),
	})

	req := httptest.NewRequest("GET", "/auth/callback?state=abc", nil)
	w := httptest.NewRecorder()
	h.Callback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCallback_AzureError(t *testing.T) {
	h := NewHandler(Config{
		ClientID:   "test-client-id",
		TenantID:   "test-tenant",
		SessionKey: []byte("test-session-key-32-bytes-long!!"),
	})

	req := httptest.NewRequest("GET", "/auth/callback?state=abc&error=access_denied&error_description=user+denied", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "abc"})
	w := httptest.NewRecorder()
	h.Callback(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestLogout_Disabled(t *testing.T) {
	h := NewHandler(Config{})

	req := httptest.NewRequest("GET", "/auth/logout", nil)
	w := httptest.NewRecorder()
	h.Logout(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if loc := w.Header().Get("Location"); loc != "/" {
		t.Errorf("redirect = %q, want /", loc)
	}
}

func TestLogout_Enabled(t *testing.T) {
	h := NewHandler(Config{
		ClientID:    "test-client-id",
		TenantID:    "test-tenant",
		RedirectURL: "http://localhost:8080/auth/callback",
		SessionKey:  []byte("test-session-key-32-bytes-long!!"),
	})

	req := httptest.NewRequest("GET", "/auth/logout", nil)
	w := httptest.NewRecorder()
	h.Logout(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusFound)
	}

	location := w.Header().Get("Location")
	if location == "" {
		t.Error("expected redirect location")
	}

	// Should clear session cookie
	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "session" && c.MaxAge != -1 {
			t.Error("session cookie should be cleared (MaxAge = -1)")
		}
	}
}

func TestCreateSession_And_GetUserFromSession(t *testing.T) {
	sessionKey := []byte("test-session-key-32-bytes-long!!")
	h := &Handler{sessionKey: sessionKey, enabled: true}

	user := &UserInfo{Name: "Test User", Email: "test@example.com"}

	w := httptest.NewRecorder()
	if err := h.createSession(w, user); err != nil {
		t.Fatalf("createSession() error = %v", err)
	}

	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie set")
	}

	// Verify we can read the session back
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessionCookie)
	gotUser, err := h.getUserFromSession(req)
	if err != nil {
		t.Fatalf("getUserFromSession() error = %v", err)
	}
	if gotUser.Name != "Test User" {
		t.Errorf("name = %q, want %q", gotUser.Name, "Test User")
	}
	if gotUser.Email != "test@example.com" {
		t.Errorf("email = %q, want %q", gotUser.Email, "test@example.com")
	}
}

func TestGetUserFromSession_NoCookie(t *testing.T) {
	h := &Handler{sessionKey: []byte("key"), enabled: true}
	req := httptest.NewRequest("GET", "/", nil)
	_, err := h.getUserFromSession(req)
	if err == nil {
		t.Error("expected error when no cookie")
	}
}

func TestGetUserFromSession_WrongSigningMethod(t *testing.T) {
	h := &Handler{sessionKey: []byte("test-key"), enabled: true}

	// Create a token signed with a different key
	claims := jwt.MapClaims{"name": "test", "exp": time.Now().Add(1 * time.Hour).Unix()}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte("different-key"))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: tokenString})
	_, err := h.getUserFromSession(req)
	if err == nil {
		t.Error("expected error for wrong key")
	}
}

func TestParseIDToken(t *testing.T) {
	claims := map[string]interface{}{
		"name":               "John Doe",
		"preferred_username": "john@example.com",
		"email":              "john@example.com",
	}
	payload, _ := json.Marshal(claims)
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	idToken := "header." + encoded + ".signature"

	user, err := parseIDToken(idToken)
	if err != nil {
		t.Fatalf("parseIDToken() error = %v", err)
	}
	if user.Name != "John Doe" {
		t.Errorf("Name = %q", user.Name)
	}
	if user.Email != "john@example.com" {
		t.Errorf("Email = %q", user.Email)
	}
}

func TestParseIDToken_EmailFallback(t *testing.T) {
	claims := map[string]interface{}{
		"name":               "Jane Doe",
		"preferred_username": "jane@example.com",
	}
	payload, _ := json.Marshal(claims)
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	idToken := "header." + encoded + ".signature"

	user, err := parseIDToken(idToken)
	if err != nil {
		t.Fatalf("parseIDToken() error = %v", err)
	}
	if user.Email != "jane@example.com" {
		t.Errorf("Email = %q, want preferred_username fallback", user.Email)
	}
}

func TestParseIDToken_InvalidFormat(t *testing.T) {
	_, err := parseIDToken("not.a.valid.token.format")
	if err == nil {
		t.Error("expected error for invalid format")
	}

	_, err = parseIDToken("onlyonepart")
	if err == nil {
		t.Error("expected error for single-part token")
	}
}

func TestParseIDToken_InvalidBase64(t *testing.T) {
	_, err := parseIDToken("header.!!!invalid-base64!!!.sig")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestParseIDToken_InvalidJSON(t *testing.T) {
	encoded := base64.RawURLEncoding.EncodeToString([]byte("not json"))
	_, err := parseIDToken("header." + encoded + ".sig")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestCallback_CodeExchangeError(t *testing.T) {
	// Mock token endpoint that returns an error
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer tokenServer.Close()

	sessionKey := []byte("test-session-key-32-bytes-long!!")
	h := &Handler{
		oauthConfig: &oauth2.Config{
			ClientID:     "test-id",
			ClientSecret: "test-secret",
			Endpoint: oauth2.Endpoint{
				TokenURL: tokenServer.URL + "/token",
			},
			RedirectURL: "http://localhost:8080/auth/callback",
		},
		sessionKey: sessionKey,
		tenantID:   "test-tenant",
		enabled:    true,
	}

	req := httptest.NewRequest("GET", "/auth/callback?state=abc&code=test-code", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "abc"})
	w := httptest.NewRecorder()
	h.Callback(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestCallback_SuccessFlow(t *testing.T) {
	// Build a mock ID token payload
	idClaims := map[string]interface{}{
		"name":               "Test User",
		"preferred_username": "test@example.com",
		"email":              "test@example.com",
	}
	payload, _ := json.Marshal(idClaims)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	fakeIDToken := "header." + encodedPayload + ".signature"

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "fake-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     fakeIDToken,
		})
	}))
	defer tokenServer.Close()

	sessionKey := []byte("test-session-key-32-bytes-long!!")
	h := &Handler{
		oauthConfig: &oauth2.Config{
			ClientID:     "test-id",
			ClientSecret: "test-secret",
			Endpoint: oauth2.Endpoint{
				TokenURL: tokenServer.URL + "/token",
			},
			RedirectURL: "http://localhost:8080/auth/callback",
		},
		sessionKey: sessionKey,
		tenantID:   "test-tenant",
		enabled:    true,
	}

	req := httptest.NewRequest("GET", "/auth/callback?state=abc&code=auth-code", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "abc"})
	w := httptest.NewRecorder()
	h.Callback(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if loc := w.Header().Get("Location"); loc != "/" {
		t.Errorf("redirect = %q, want /", loc)
	}

	// Verify that a session cookie was set (the createSession call succeeded)
	cookies := w.Result().Cookies()
	hasSession := false
	for _, c := range cookies {
		if c.Name == "session" && c.Value != "" {
			hasSession = true
		}
	}
	if !hasSession {
		t.Error("expected session cookie to be set")
	}
}

func TestCallback_WithRedirectCookie(t *testing.T) {
	idClaims := map[string]interface{}{
		"name":  "User",
		"email": "user@test.com",
	}
	payload, _ := json.Marshal(idClaims)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	fakeIDToken := "h." + encodedPayload + ".s"

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     fakeIDToken,
		})
	}))
	defer tokenServer.Close()

	h := &Handler{
		oauthConfig: &oauth2.Config{
			ClientID:     "id",
			ClientSecret: "secret",
			Endpoint:     oauth2.Endpoint{TokenURL: tokenServer.URL},
			RedirectURL:  "http://localhost:8080/auth/callback",
		},
		sessionKey: []byte("test-session-key-32-bytes-long!!"),
		tenantID:   "tenant",
		enabled:    true,
	}

	req := httptest.NewRequest("GET", "/auth/callback?state=xyz&code=code", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "xyz"})
	req.AddCookie(&http.Cookie{Name: "auth_redirect", Value: "/dashboard"})
	w := httptest.NewRecorder()
	h.Callback(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if loc := w.Header().Get("Location"); loc != "/dashboard" {
		t.Errorf("redirect = %q, want /dashboard", loc)
	}
}

func TestCallback_NoIDToken(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			// no id_token
		})
	}))
	defer tokenServer.Close()

	h := &Handler{
		oauthConfig: &oauth2.Config{
			ClientID:     "id",
			ClientSecret: "secret",
			Endpoint:     oauth2.Endpoint{TokenURL: tokenServer.URL},
			RedirectURL:  "http://localhost:8080/auth/callback",
		},
		sessionKey: []byte("test-session-key-32-bytes-long!!"),
		tenantID:   "tenant",
		enabled:    true,
	}

	req := httptest.NewRequest("GET", "/auth/callback?state=abc&code=code", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "abc"})
	w := httptest.NewRecorder()
	h.Callback(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestCallback_InvalidIDToken(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     "invalid",
		})
	}))
	defer tokenServer.Close()

	h := &Handler{
		oauthConfig: &oauth2.Config{
			ClientID:     "id",
			ClientSecret: "secret",
			Endpoint:     oauth2.Endpoint{TokenURL: tokenServer.URL},
			RedirectURL:  "http://localhost:8080/auth/callback",
		},
		sessionKey: []byte("test-session-key-32-bytes-long!!"),
		tenantID:   "tenant",
		enabled:    true,
	}

	req := httptest.NewRequest("GET", "/auth/callback?state=abc&code=code", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "abc"})
	w := httptest.NewRecorder()
	h.Callback(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestGetUserFromContext_WithUser(t *testing.T) {
	user := &UserInfo{Name: "Test", Email: "test@test.com"}
	ctx := context.WithValue(context.Background(), userContextKey, user)
	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)

	got := GetUserFromContext(req)
	if got == nil {
		t.Fatal("expected user from context")
	}
	if got.Name != "Test" {
		t.Errorf("Name = %q", got.Name)
	}
}

func TestGetUserFromContext_NoUser(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	got := GetUserFromContext(req)
	if got != nil {
		t.Error("expected nil when no user in context")
	}
}

func TestGenerateRandomString(t *testing.T) {
	s1, err := generateRandomString(16)
	if err != nil {
		t.Fatalf("generateRandomString() error = %v", err)
	}
	s2, err := generateRandomString(16)
	if err != nil {
		t.Fatalf("generateRandomString() error = %v", err)
	}

	if s1 == "" || s2 == "" {
		t.Error("generated strings should not be empty")
	}
	if s1 == s2 {
		t.Error("two random strings should differ")
	}
}
