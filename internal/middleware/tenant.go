package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rishabharidas/restolink/internal/models"
)

type contextKey string

const (
	TenantContextKey contextKey = "tenant"
	UserContextKey   contextKey = "user"
)

func TenantMiddleware(pool *pgxpool.Pool, rootDomain string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tenantSlug string

			// 1. Check Header: X-Tenant-Slug
			if slug := r.Header.Get("X-Tenant-Slug"); slug != "" {
				tenantSlug = slug
			}

			// 2. Check Query Param: ?tenant=...
			if tenantSlug == "" {
				tenantSlug = r.URL.Query().Get("tenant")
			}

			// 3. Check Subdomain (e.g. test-resto.resto.local)
			if tenantSlug == "" {
				host := r.Host
				if colon := strings.Index(host, ":"); colon != -1 {
					host = host[:colon]
				}
				if strings.HasSuffix(host, rootDomain) && host != rootDomain {
					tenantSlug = strings.TrimSuffix(host, "."+rootDomain)
				}
			}

			// Fallback: Default to demo test tenant if none specified (for local dev)
			if tenantSlug == "" {
				tenantSlug = "test-resto"
			}

			// Query tenant from DB
			var t models.Tenant
			query := `
				SELECT id, slug, name, custom_domain, phone, email, address, branding, settings, is_active, created_at, updated_at
				FROM restoapp.tenants
				WHERE slug = $1 AND is_active = true
			`
			err := pool.QueryRow(r.Context(), query, tenantSlug).Scan(
				&t.ID, &t.Slug, &t.Name, &t.CustomDomain, &t.Phone, &t.Email, &t.Address,
				&t.BrandingRaw, &t.SettingsRaw, &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
			)

			if err == nil {
				// Inject tenant into request context
				ctx := context.WithValue(r.Context(), TenantContextKey, &t)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// If tenant lookup fails, continue or return 404
			next.ServeHTTP(w, r)
		})
	}
}

func GetTenantFromContext(ctx context.Context) *models.Tenant {
	if val, ok := ctx.Value(TenantContextKey).(*models.Tenant); ok {
		return val
	}
	return nil
}

func GetTenantID(ctx context.Context) uuid.UUID {
	if t := GetTenantFromContext(ctx); t != nil {
		return t.ID
	}
	return uuid.Nil
}
