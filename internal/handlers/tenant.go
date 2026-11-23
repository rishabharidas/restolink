package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/rishabharidas/restolink/internal/middleware"
)

type TenantHandler struct{}

func NewTenantHandler() *TenantHandler {
	return &TenantHandler{}
}

func (h *TenantHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.GetTenantFromContext(r.Context())
	if tenant == nil {
		http.Error(w, `{"error":"not_found","message":"tenant not found"}`, http.StatusNotFound)
		return
	}

	// Unmarshal jsonb fields if not already populated
	if len(tenant.BrandingRaw) > 0 {
		_ = json.Unmarshal(tenant.BrandingRaw, &tenant.Branding)
	}
	if len(tenant.SettingsRaw) > 0 {
		_ = json.Unmarshal(tenant.SettingsRaw, &tenant.Settings)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tenant)
}
