package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rishabharidas/restolink/internal/middleware"
	"github.com/rishabharidas/restolink/internal/models"
)

type MenuHandler struct {
	pool *pgxpool.Pool
}

func NewMenuHandler(pool *pgxpool.Pool) *MenuHandler {
	return &MenuHandler{pool: pool}
}

func (h *MenuHandler) GetMenu(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		http.Error(w, `{"error":"not_found","message":"tenant not found"}`, http.StatusNotFound)
		return
	}

	// 1. Fetch categories
	catRows, err := h.pool.Query(r.Context(), `
		SELECT id, tenant_id, name, COALESCE(description, ''), display_order, is_active
		FROM restoapp.menu_categories
		WHERE tenant_id = $1 AND is_active = true
		ORDER BY display_order ASC
	`, tenantID)
	if err != nil {
		http.Error(w, `{"error":"database_error","message":"failed to fetch categories"}`, http.StatusInternalServerError)
		return
	}
	defer catRows.Close()

	categoryMap := make(map[uuid.UUID]*models.MenuCategory)
	var categories []*models.MenuCategory

	for catRows.Next() {
		var c models.MenuCategory
		if err := catRows.Scan(&c.ID, &c.TenantID, &c.Name, &c.Description, &c.DisplayOrder, &c.IsActive); err != nil {
			continue
		}
		c.Items = make([]models.MenuItem, 0)
		categoryMap[c.ID] = &c
		categories = append(categories, &c)
	}

	// 2. Fetch menu items
	itemRows, err := h.pool.Query(r.Context(), `
		SELECT id, tenant_id, category_id, name, COALESCE(description, ''), price, food_type,
		       COALESCE(image_url, ''), is_available, kitchen_station, preparation_time_mins, created_at
		FROM restoapp.menu_items
		WHERE tenant_id = $1 AND is_available = true
		ORDER BY name ASC
	`, tenantID)
	if err == nil {
		defer itemRows.Close()
		for itemRows.Next() {
			var it models.MenuItem
			if err := itemRows.Scan(&it.ID, &it.TenantID, &it.CategoryID, &it.Name, &it.Description,
				&it.Price, &it.FoodType, &it.ImageURL, &it.IsAvailable, &it.KitchenStation,
				&it.PreparationTimeMins, &it.CreatedAt); err != nil {
				continue
			}
			if cat, ok := categoryMap[it.CategoryID]; ok {
				cat.Items = append(cat.Items, it)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(categories)
}

func (h *MenuHandler) ResolveTableQR(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		token = r.URL.Query().Get("token")
	}

	var table models.RestaurantTable
	query := `
		SELECT id, tenant_id, outlet_id, table_number, qr_code_token, capacity, status, active_order_id
		FROM restoapp.restaurant_tables
		WHERE qr_code_token = $1
	`
	err := h.pool.QueryRow(r.Context(), query, token).Scan(
		&table.ID, &table.TenantID, &table.OutletID, &table.TableNumber, &table.QRCodeToken,
		&table.Capacity, &table.Status, &table.ActiveOrderID,
	)
	if err != nil {
		http.Error(w, `{"error":"not_found","message":"invalid or expired table QR code"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(table)
}
