package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
)

type contextKey string

const userContextKey contextKey = "user"

// UserInfo represents the authenticated user extracted from Azure AD tokens.
type UserInfo struct {
	Name  string
	Email string
}

// Config holds the Azure AD authentication configuration.
type Config struct {
	ClientID     string
	ClientSecret string
	TenantID     string
	RedirectURL  string
	SessionKey   []byte
}

// Handler manages Azure AD OAuth2 authentication.
type Handler struct {
	oauthConfig *oauth2.Config
	sessionKey  []byte
	tenantID    string
	enabled     bool
}

// NewHandler creates a new auth handler from the provided config.
// If required fields are missing, authentication is disabled gracefully.
func NewHandler(cfg Config) *Handler {
	if cfg.ClientID == "" || cfg.TenantID == "" {
		log.Println("Azure AD authentication disabled (set AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, AZURE_TENANT_ID to enable)")
		return &Handler{enabled: false}
	}

	if cfg.RedirectURL == "" {
		cfg.RedirectURL = "http://localhost:8080/auth/callback"
	}

	if len(cfg.SessionKey) == 0 {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			log.Fatalf("Failed to generate session secret: %v", err)
		}
		cfg.SessionKey = secret
		log.Println("WARNING: SESSION_SECRET not set, using random secret (sessions won't persist across restarts)")
	}

	oauthConfig := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:  fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize", cfg.TenantID),
			TokenURL: fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", cfg.TenantID),
		},
		Scopes: []string{"openid", "profile", "email"},
	}

	log.Printf("Azure AD authentication enabled (Tenant: %s, Client: %s)", cfg.TenantID, cfg.ClientID)

	return &Handler{
		oauthConfig: oauthConfig,
		sessionKey:  cfg.SessionKey,
		tenantID:    cfg.TenantID,
		enabled:     true,
	}
}

// IsEnabled returns whether Azure AD authentication is configured and active.
func (h *Handler) IsEnabled() bool {
	return h.enabled
}

// RequireAuth is middleware that protects routes with Azure AD authentication.
// Unauthenticated users are redirected to the login page.
func (h *Handler) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	if !h.enabled {
		return next
	}

	return func(w http.ResponseWriter, r *http.Request) {
		user, err := h.getUserFromSession(r)
		if err != nil {
			// Remember where the user wanted to go
			http.SetCookie(w, &http.Cookie{
				Name:     "auth_redirect",
				Value:    r.URL.String(),
				Path:     "/",
				MaxAge:   300,
				HttpOnly: true,
				Secure:   true,
				SameSite: http.SameSiteLaxMode,
			})
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next(w, r.WithContext(ctx))
	}
}

// Login initiates the Azure AD OAuth2 authorization code flow.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if !h.enabled {
		http.Error(w, "Authentication not configured", http.StatusServiceUnavailable)
		return
	}

	state, err := generateRandomString(16)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	url := h.oauthConfig.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusFound)
}

// Callback handles the OAuth2 redirect from Azure AD after user login.
func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	if !h.enabled {
		http.Error(w, "Authentication not configured", http.StatusServiceUnavailable)
		return
	}

	// Verify state to prevent CSRF
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	// Clear the state cookie
	http.SetCookie(w, &http.Cookie{
		HttpOnly: true,
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   true,
	})

	// Check for errors from Azure AD (e.g. user denied consent)
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		errDesc := r.URL.Query().Get("error_description")
		log.Printf("Azure AD auth error: %s - %s", errMsg, errDesc)
		http.Error(w, fmt.Sprintf("Authentication error: %s", errMsg), http.StatusForbidden)
		return
	}

	// Exchange authorization code for tokens
	code := r.URL.Query().Get("code")
	token, err := h.oauthConfig.Exchange(r.Context(), code)
	if err != nil {
		log.Printf("Token exchange error: %v", err)
		http.Error(w, "Failed to exchange authorization code", http.StatusInternalServerError)
		return
	}

	// Extract user info from the ID token
	idToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "No ID token in response", http.StatusInternalServerError)
		return
	}

	user, err := parseIDToken(idToken)
	if err != nil {
		log.Printf("Error parsing ID token: %v", err)
		http.Error(w, "Failed to parse identity", http.StatusInternalServerError)
		return
	}

	// Create a session cookie
	if err := h.createSession(w, user); err != nil {
		log.Printf("Error creating session: %v", err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	// Redirect to the originally requested URL, or the dashboard root
	redirectURL := "/"
	if cookie, err := r.Cookie("auth_redirect"); err == nil && cookie.Value != "" {
		redirectURL = cookie.Value
		http.SetCookie(w, &http.Cookie{
			HttpOnly: true,
			Name:     "auth_redirect",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			Secure:   true,
		})
	}

	allowedHosts := []string{
		"https://trusted1.example.com/",
		"https://trusted2.example.com/",
	}

	if !slices.Contains(allowedHosts, redirectURL) {
		http.Error(w, "Invalid redirect URL", http.StatusForbidden)
		return
	}

	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// Logout clears the session and optionally redirects to Azure AD logout.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	// Clear the session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
	})

	if h.enabled {
		// Build the post-logout redirect URI (strip /auth/callback from the redirect URL)
		baseURL := h.oauthConfig.RedirectURL
		if idx := strings.LastIndex(baseURL, "/auth/"); idx != -1 {
			baseURL = baseURL[:idx]
		}
		logoutURL := fmt.Sprintf(
			"https://login.microsoftonline.com/%s/oauth2/v2.0/logout?post_logout_redirect_uri=%s",
			h.tenantID, baseURL,
		)
		http.Redirect(w, r, logoutURL, http.StatusFound)
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

// createSession creates a signed JWT session cookie for the authenticated user.
func (h *Handler) createSession(w http.ResponseWriter, user *UserInfo) error {
	claims := jwt.MapClaims{
		"name":  user.Name,
		"email": user.Email,
		"exp":   time.Now().Add(8 * time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.sessionKey)
	if err != nil {
		return fmt.Errorf("signing session token: %w", err)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    tokenString,
		Path:     "/",
		MaxAge:   28800, // 8 hours
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	})

	return nil
}

// getUserFromSession validates the session cookie and returns the user info.
func (h *Handler) getUserFromSession(r *http.Request) (*UserInfo, error) {
	cookie, err := r.Cookie("session")
	if err != nil {
		return nil, fmt.Errorf("no session cookie")
	}

	token, err := jwt.Parse(cookie.Value, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return h.sessionKey, nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid session: %v", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims format")
	}

	name, _ := claims["name"].(string)
	email, _ := claims["email"].(string)

	return &UserInfo{
		Name:  name,
		Email: email,
	}, nil
}

// parseIDToken extracts user information from an Azure AD ID token.
// The token signature is not verified here because it was received directly
// from Azure AD via the secure OAuth2 token exchange (server-to-server).
func parseIDToken(idToken string) (*UserInfo, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid ID token format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decoding ID token payload: %w", err)
	}

	var claims struct {
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
		Email             string `json:"email"`
	}

	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("parsing ID token claims: %w", err)
	}

	email := claims.Email
	if email == "" {
		email = claims.PreferredUsername
	}

	return &UserInfo{
		Name:  claims.Name,
		Email: email,
	}, nil
}

// generateRandomString creates a cryptographically random URL-safe string.
func generateRandomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// GetUserFromContext retrieves the authenticated user from the request context.
// Returns nil if no user is present (e.g. auth is disabled).
func GetUserFromContext(r *http.Request) *UserInfo {
	user, _ := r.Context().Value(userContextKey).(*UserInfo)
	return user
}
