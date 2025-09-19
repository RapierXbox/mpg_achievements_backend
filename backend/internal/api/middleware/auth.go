package middleware

import (
	"backend/internal/service"
	"backend/pkg/config"
	"backend/pkg/utils"

	"context"
	"net/http"
	"strings"

	"github.com/gocql/gocql"
)

// AuthMiddleware validates jwts and injects user context
func AuthMiddleware(cfg *config.Config, sessionService *service.SessionService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				// skip authentication for public endpoints
				if isPublicRoute(r.URL.Path) {
					next.ServeHTTP(w, r)
					return
				}

				// extract token from authorization header
				authHeader := r.Header.Get("Authorization")
				if authHeader == "" {
					http.Error(w, "authorization header required", http.StatusUnauthorized)
					return
				}

				// validate token format
				tokenParts := strings.Split(authHeader, " ")
				if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
					http.Error(w, "invalid authorization format", http.StatusBadRequest)
					return
				}
				token := tokenParts[1]

				// parse and validate token
				claims, err := utils.ParseToken(token, []byte(cfg.JWTSecret))
				if err != nil {
					http.Error(w, "invalid token - "+err.Error(), http.StatusForbidden)
					return
				}

				// extract user ID from claims
				userID, err := gocql.ParseUUID(claims["sub"].(string))
				if err != nil {
					http.Error(w, "invalid user claim", http.StatusForbidden)
					return
				}

				deviceID, err := gocql.ParseUUID(r.Header.Get("X-Device-ID"))
				if err != nil {
					http.Error(w, "invalid device ID - "+err.Error(), http.StatusBadRequest)
					return
				}

				valid, err := sessionService.CheckSession(userID, deviceID)
				if err != nil {
					http.Error(w, "invalid session - "+err.Error(), http.StatusUnauthorized)
					return
				}
				if !valid {
					http.Error(w, "invalid session", http.StatusUnauthorized)
					return
				}

				// inject user ID into request context
				ctx := context.WithValue(r.Context(), "userID", userID)
				next.ServeHTTP(w, r.WithContext(ctx))
			})
	}
}

// check if route doesnt require authentication
func isPublicRoute(path string) bool {
	publicRoutes := map[string]bool{
		"/login":       true,
		"/register":    true,
		"/healthcheck": true,
	}
	return publicRoutes[path]
}
