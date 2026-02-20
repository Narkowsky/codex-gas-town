package web

import (
	"net/http"
	"net/url"
	"os"
	"strings"
)

// applyLocalCORS applies localhost-only CORS policy.
// Returns false when the request origin is explicitly disallowed.
func applyLocalCORS(w http.ResponseWriter, r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		// Non-browser clients may omit Origin.
		return true
	}
	if !isLocalOrigin(origin) {
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-GT-Dashboard-Token")
	return true
}

func isLocalOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func dashboardToken() string {
	return strings.TrimSpace(os.Getenv("GT_DASHBOARD_TOKEN"))
}

func requestHasDashboardToken(r *http.Request, expectedToken string) bool {
	if expectedToken == "" {
		return true
	}
	token := strings.TrimSpace(r.Header.Get("X-GT-Dashboard-Token"))
	if token == "" {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			token = strings.TrimSpace(auth[len("Bearer "):])
		}
	}
	return token == expectedToken
}
