package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rishabharidas/restolink/internal/middleware"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	pool      *pgxpool.Pool
	jwtSecret string
}

func NewAuthHandler(pool *pgxpool.Pool, jwtSecret string) *AuthHandler {
	return &AuthHandler{pool: pool, jwtSecret: jwtSecret}
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad_request","message":"invalid credentials format"}`, http.StatusBadRequest)
		return
	}

	var user struct {
		ID           uuid.UUID
		TenantID     *uuid.UUID
		PasswordHash string
		RoleName     string
		FirstName    string
		LastName     string
	}

	query := `
		SELECT u.id, u.tenant_id, u.password_hash, r.name, u.first_name, u.last_name
		FROM restoapp.users u
		JOIN restoapp.roles r ON u.role_id = r.id
		WHERE u.phone = $1 OR u.email = $1
		LIMIT 1
	`
	err := h.pool.QueryRow(r.Context(), query, req.Username).Scan(
		&user.ID, &user.TenantID, &user.PasswordHash, &user.RoleName, &user.FirstName, &user.LastName,
	)
	if err != nil {
		http.Error(w, `{"error":"unauthorized","message":"invalid username or password"}`, http.StatusUnauthorized)
		return
	}

	// Verify bcrypt hash
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		http.Error(w, `{"error":"unauthorized","message":"invalid username or password"}`, http.StatusUnauthorized)
		return
	}

	// Generate JWT
	claims := middleware.AuthClaims{
		UserID:   user.ID,
		TenantID: user.TenantID,
		Role:     user.RoleName,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(h.jwtSecret))
	if err != nil {
		http.Error(w, `{"error":"server_error","message":"failed to sign auth token"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"token": tokenString,
		"user": map[string]interface{}{
			"id":         user.ID,
			"first_name": user.FirstName,
			"last_name":  user.LastName,
			"role":       user.RoleName,
		},
	})
}
