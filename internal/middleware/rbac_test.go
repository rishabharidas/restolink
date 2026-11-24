package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/rishabharidas/restolink/internal/models"
)

func TestRequireRole_AllowsAuthorizedRole(t *testing.T) {
	tenantID := uuid.New()
	userClaims := &AuthClaims{
		UserID:   uuid.New(),
		TenantID: &tenantID,
		Role:     "chef",
	}

	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	mw := RequireRole("chef", "manager")(testHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	// Inject tenant and auth user into context
	ctx := context.WithValue(req.Context(), TenantContextKey, &models.Tenant{ID: tenantID})
	ctx = context.WithValue(ctx, UserContextKey, userClaims)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected HTTP 200, got %d", rr.Code)
	}
	if !handlerCalled {
		t.Errorf("Expected inner handler to be called")
	}
}

func TestRequireRole_RejectsUnauthorizedRole(t *testing.T) {
	tenantID := uuid.New()
	userClaims := &AuthClaims{
		UserID:   uuid.New(),
		TenantID: &tenantID,
		Role:     "waiter", // Waiter trying to access chef-only endpoint
	}

	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	mw := RequireRole("chef", "manager")(testHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	ctx := context.WithValue(req.Context(), TenantContextKey, &models.Tenant{ID: tenantID})
	ctx = context.WithValue(ctx, UserContextKey, userClaims)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected HTTP 403 Forbidden, got %d", rr.Code)
	}
	if handlerCalled {
		t.Errorf("Inner handler should NOT have been called for unauthorized role")
	}
}

func TestRequireRole_RejectsCrossTenantAccess(t *testing.T) {
	tenantA := uuid.New()
	tenantB := uuid.New()

	// User belongs to Tenant B, but request is for Tenant A
	userClaims := &AuthClaims{
		UserID:   uuid.New(),
		TenantID: &tenantB,
		Role:     "owner",
	}

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := RequireRole("owner")(testHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	ctx := context.WithValue(req.Context(), TenantContextKey, &models.Tenant{ID: tenantA})
	ctx = context.WithValue(ctx, UserContextKey, userClaims)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected HTTP 403 Forbidden for cross-tenant access, got %d", rr.Code)
	}
}

func TestRequireRole_AllowsSuperadminAcrossTenants(t *testing.T) {
	tenantA := uuid.New()

	// Superadmin has no specific tenant
	userClaims := &AuthClaims{
		UserID:   uuid.New(),
		TenantID: nil,
		Role:     "superadmin",
	}

	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	mw := RequireRole("owner")(testHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	ctx := context.WithValue(req.Context(), TenantContextKey, &models.Tenant{ID: tenantA})
	ctx = context.WithValue(ctx, UserContextKey, userClaims)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected superadmin to be allowed (HTTP 200), got %d", rr.Code)
	}
	if !handlerCalled {
		t.Errorf("Expected handler to be called for superadmin")
	}
}
