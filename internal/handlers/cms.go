package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rishabharidas/restolink/internal/errors"
	"github.com/rishabharidas/restolink/internal/middleware"
	"github.com/rishabharidas/restolink/internal/models"
)

type CMSHandler struct {
	pool *pgxpool.Pool
}

func NewCMSHandler(pool *pgxpool.Pool) *CMSHandler {
	return &CMSHandler{pool: pool}
}

// GetPage fetches a published page and all its enabled sections for the tenant storefront.
func (h *CMSHandler) GetPage(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		slug = "home"
	}

	ctx := r.Context()
	var page models.WebsitePage

	// 1. Fetch page metadata
	queryPage := `
		SELECT id, tenant_id, slug, title, is_published, COALESCE(meta_description, ''), created_at, updated_at
		FROM restoapp.website_pages
		WHERE tenant_id = $1 AND slug = $2
	`
	err := h.pool.QueryRow(ctx, queryPage, tenantID, slug).Scan(
		&page.ID, &page.TenantID, &page.Slug, &page.Title, &page.IsPublished,
		&page.MetaDescription, &page.CreatedAt, &page.UpdatedAt,
	)
	if err != nil {
		// Auto-provision home page if not present
		page = models.WebsitePage{
			ID:          uuid.New(),
			TenantID:    tenantID,
			Slug:        slug,
			Title:       "Restaurant Storefront",
			IsPublished: true,
		}
		_, _ = h.pool.Exec(ctx, `
			INSERT INTO restoapp.website_pages (id, tenant_id, slug, title, is_published)
			VALUES ($1, $2, $3, $4, true)
			ON CONFLICT (tenant_id, slug) DO NOTHING
		`, page.ID, tenantID, slug, page.Title)
	}

	// 2. Fetch sections ordered by display_order
	querySections := `
		SELECT id, page_id, tenant_id, section_type, display_order, is_enabled, content, settings, created_at, updated_at
		FROM restoapp.website_sections
		WHERE tenant_id = $1 AND (is_enabled = true OR $2 = true)
		ORDER BY display_order ASC
	`
	includeDrafts := r.URL.Query().Get("all") == "true"
	rows, err := h.pool.Query(ctx, querySections, tenantID, includeDrafts)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var s models.WebsiteSection
			if err := rows.Scan(
				&s.ID, &s.PageID, &s.TenantID, &s.SectionType, &s.DisplayOrder,
				&s.IsEnabled, &s.Content, &s.Settings, &s.CreatedAt, &s.UpdatedAt,
			); err == nil {
				page.Sections = append(page.Sections, s)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"page":    page,
	})
}

// UpdateLandingMode updates the tenant's landing mode (DEFAULT vs CUSTOM_HTML) and custom HTML.
type UpdateLandingModeRequest struct {
	LandingMode       string `json:"landing_mode"`
	CustomLandingHTML string `json:"custom_landing_html"`
}

func (h *CMSHandler) UpdateLandingMode(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	var req UpdateLandingModeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid request body"))
		return
	}

	ctx := r.Context()

	// Update tenant branding JSON
	query := `
		UPDATE restoapp.tenants
		SET branding = jsonb_set(
			jsonb_set(COALESCE(branding, '{}'::jsonb), '{landing_mode}', to_jsonb($1::text)),
			'{custom_landing_html}', to_jsonb($2::text)
		),
		updated_at = NOW()
		WHERE id = $3
	`
	_, err := h.pool.Exec(ctx, query, req.LandingMode, req.CustomLandingHTML, tenantID)
	if err != nil {
		errors.WriteError(w, errors.Internal("Failed to update tenant landing configuration"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":             true,
		"landing_mode":        req.LandingMode,
		"custom_landing_html": req.CustomLandingHTML,
	})
}

// ---------------------------------------------------------------------
// Dynamic Section CRUD & Reordering
// ---------------------------------------------------------------------

type SectionRequest struct {
	SectionType  string                 `json:"section_type"` // HERO, ABOUT, MENU_HIGHLIGHTS, CHEF_STORY, AMBIENCE_GALLERY, RESERVATION_CALLOUT, TESTIMONIALS, CONTACT_HOURS
	DisplayOrder int                    `json:"display_order"`
	IsEnabled    bool                   `json:"is_enabled"`
	Content      map[string]interface{} `json:"content"`
	Settings     map[string]interface{} `json:"settings"`
}

func (h *CMSHandler) AddSection(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		slug = "home"
	}

	var req SectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SectionType == "" {
		errors.WriteError(w, errors.BadRequest("Section type is required"))
		return
	}

	ctx := r.Context()

	// Ensure page exists
	var pageID uuid.UUID
	err := h.pool.QueryRow(ctx, "SELECT id FROM restoapp.website_pages WHERE tenant_id = $1 AND slug = $2", tenantID, slug).Scan(&pageID)
	if err != nil {
		pageID = uuid.New()
		_, _ = h.pool.Exec(ctx, "INSERT INTO restoapp.website_pages (id, tenant_id, slug, title, is_published) VALUES ($1, $2, $3, 'Storefront', true)", pageID, tenantID, slug)
	}

	sectionID := uuid.New()
	contentBytes, _ := json.Marshal(req.Content)
	settingsBytes, _ := json.Marshal(req.Settings)

	query := `
		INSERT INTO restoapp.website_sections (
			id, page_id, tenant_id, section_type, display_order, is_enabled, content, settings, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
	`
	_, err = h.pool.Exec(ctx, query, sectionID, pageID, tenantID, req.SectionType, req.DisplayOrder, req.IsEnabled, contentBytes, settingsBytes)
	if err != nil {
		errors.WriteError(w, errors.Internal("Failed to add website section"))
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"section_id": sectionID,
		"section":    req,
	})
}

func (h *CMSHandler) UpdateSection(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	sectionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid section ID"))
		return
	}

	var req SectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid request body"))
		return
	}

	contentBytes, _ := json.Marshal(req.Content)
	settingsBytes, _ := json.Marshal(req.Settings)

	query := `
		UPDATE restoapp.website_sections
		SET section_type = $1, display_order = $2, is_enabled = $3,
		    content = $4, settings = $5, updated_at = NOW()
		WHERE id = $6 AND tenant_id = $7
	`
	res, err := h.pool.Exec(r.Context(), query, req.SectionType, req.DisplayOrder, req.IsEnabled, contentBytes, settingsBytes, sectionID, tenantID)
	if err != nil || res.RowsAffected() == 0 {
		errors.WriteError(w, errors.NotFound("Section not found for this tenant"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"section_id": sectionID,
		"section":    req,
	})
}

func (h *CMSHandler) DeleteSection(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	sectionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid section ID"))
		return
	}

	res, err := h.pool.Exec(r.Context(), "DELETE FROM restoapp.website_sections WHERE id = $1 AND tenant_id = $2", sectionID, tenantID)
	if err != nil || res.RowsAffected() == 0 {
		errors.WriteError(w, errors.NotFound("Section not found for this tenant"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"section_id": sectionID,
	})
}

type ReorderSectionsRequest struct {
	SectionIDs []uuid.UUID `json:"section_ids"`
}

func (h *CMSHandler) ReorderSections(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	var req ReorderSectionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.SectionIDs) == 0 {
		errors.WriteError(w, errors.BadRequest("section_ids array is required"))
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		errors.WriteError(w, errors.Internal("Failed to begin transaction"))
		return
	}
	defer tx.Rollback(ctx)

	for i, sid := range req.SectionIDs {
		_, err := tx.Exec(ctx, `
			UPDATE restoapp.website_sections
			SET display_order = $1, updated_at = NOW()
			WHERE id = $2 AND tenant_id = $3
		`, i+1, sid, tenantID)
		if err != nil {
			errors.WriteError(w, errors.Internal("Failed to reorder sections"))
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		errors.WriteError(w, errors.Internal("Failed to commit reorder transaction"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"section_ids": req.SectionIDs,
	})
}
