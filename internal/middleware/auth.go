package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type AuthClaims struct {
	UserID   uuid.UUID  `json:"user_id"`
	TenantID *uuid.UUID `json:"tenant_id,omitempty"`
	Role     string     `json:"role"`
	jwt.RegisteredClaims
}

func AuthMiddleware(jwtSecret string, rdb *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, `{"error":"unauthorized","message":"missing or invalid authorization header"}`, http.StatusUnauthorized)
				return
			}

			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

			// Check token blacklist in Redis
			if rdb != nil {
				isBlacklisted, _ := rdb.Exists(r.Context(), "blacklist:"+tokenStr).Result()
				if isBlacklisted > 0 {
					http.Error(w, `{"error":"unauthorized","message":"token has been revoked"}`, http.StatusUnauthorized)
					return
				}
			}

			claims := &AuthClaims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
				return []byte(jwtSecret), nil
			})

			if err != nil || !token.Valid {
				http.Error(w, `{"error":"unauthorized","message":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			// Add claims to context
			ctx := context.WithValue(r.Context(), UserContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetAuthUser(ctx context.Context) *AuthClaims {
	if val, ok := ctx.Value(UserContextKey).(*AuthClaims); ok {
		return val
	}
	return nil
}
