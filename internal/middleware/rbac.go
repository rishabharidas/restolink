package middleware

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/rishabharidas/restolink/internal/errors"
)

// RequireRole enforces that the authenticated user possesses one of the specified roles
// and belongs to the active tenant in context (or is a platform superadmin).
func RequireRole(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetAuthUser(r.Context())
			if user == nil {
				errors.WriteError(w, errors.Unauthorized("Authentication required for this operation"))
				return
			}

			// Superadmin bypasses tenant matching and role checks
			if user.Role == "superadmin" {
				next.ServeHTTP(w, r)
				return
			}

			// Tenant validation: Ensure user's tenant matches the active tenant context
			tenantID := GetTenantID(r.Context())
			if tenantID != uuid.Nil && user.TenantID != nil && *user.TenantID != tenantID {
				errors.WriteError(w, errors.Forbidden("User does not have access to this restaurant tenant"))
				return
			}

			// Check role match
			roleMatched := false
			for _, role := range allowedRoles {
				if user.Role == role {
					roleMatched = true
					break
				}
			}

			if !roleMatched {
				errors.WriteError(w, errors.Forbidden("Insufficient role permissions for this action"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
