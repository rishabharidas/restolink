package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rishabharidas/restolink/internal/errors"
	"github.com/rishabharidas/restolink/internal/middleware"
	"github.com/rishabharidas/restolink/internal/models"
	"github.com/rishabharidas/restolink/internal/realtime"
)

type MenuHandler struct {
	pool *pgxpool.Pool
	hub  *realtime.Hub
}

func NewMenuHandler(pool *pgxpool.Pool, hub *realtime.Hub) *MenuHandler {
	return &MenuHandler{pool: pool, hub: hub}
}

// ---------------------------------------------------------------------
// Menu Retrieval & QR Table Resolution
// ---------------------------------------------------------------------

func (h *MenuHandler) GetMenu(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
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
		errors.WriteError(w, errors.Internal("Failed to fetch menu categories"))
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

	// 2. Fetch menu items with allergens, stations, variants, and modifiers
	itemRows, err := h.pool.Query(r.Context(), `
		SELECT id, tenant_id, category_id, name, COALESCE(description, ''), price, food_type,
		       COALESCE(image_url, ''), is_available, COALESCE(kitchen_station, 'MAIN_KITCHEN'),
		       COALESCE(assigned_stations, '{"MAIN_KITCHEN"}'), COALESCE(allergens, '{}'),
		       preparation_time_mins, COALESCE(variants, '[]'::jsonb), COALESCE(modifiers, '[]'::jsonb), created_at
		FROM restoapp.menu_items
		WHERE tenant_id = $1
		ORDER BY name ASC
	`, tenantID)
	if err == nil {
		defer itemRows.Close()
		for itemRows.Next() {
			var it models.MenuItem
			var variantsRaw, modifiersRaw []byte
			if err := itemRows.Scan(
				&it.ID, &it.TenantID, &it.CategoryID, &it.Name, &it.Description,
				&it.Price, &it.FoodType, &it.ImageURL, &it.IsAvailable, &it.KitchenStation,
				&it.AssignedStations, &it.Allergens, &it.PreparationTimeMins,
				&variantsRaw, &modifiersRaw, &it.CreatedAt,
			); err != nil {
				continue
			}
			if len(variantsRaw) > 0 {
				_ = json.Unmarshal(variantsRaw, &it.Variants)
			}
			if len(modifiersRaw) > 0 {
				_ = json.Unmarshal(modifiersRaw, &it.Modifiers)
			}
			if it.AssignedStations == nil {
				it.AssignedStations = []string{it.KitchenStation}
			}
			if it.Allergens == nil {
				it.Allergens = []string{}
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
		errors.WriteError(w, errors.NotFound("Invalid or expired table QR code"))
		return
	}

	activeTenantID := middleware.GetTenantID(r.Context())
	if activeTenantID != uuid.Nil && table.TenantID != activeTenantID {
		errors.WriteError(w, errors.Forbidden("Table QR code does not belong to active restaurant"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(table)
}

// ---------------------------------------------------------------------
// Availability Toggle (86'ing)
// ---------------------------------------------------------------------

type ToggleAvailabilityRequest struct {
	IsAvailable bool `json:"is_available"`
}

func (h *MenuHandler) ToggleAvailability(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	itemIDStr := chi.URLParam(r, "id")
	itemID, err := uuid.Parse(itemIDStr)
	if err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid item ID format"))
		return
	}

	var req ToggleAvailabilityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid request body"))
		return
	}

	ctx := r.Context()
	res, err := h.pool.Exec(ctx, "UPDATE restoapp.menu_items SET is_available = $1 WHERE id = $2 AND tenant_id = $3", req.IsAvailable, itemID, tenantID)
	if err != nil || res.RowsAffected() == 0 {
		errors.WriteError(w, errors.NotFound("Menu item not found for this tenant"))
		return
	}

	h.hub.BroadcastToRoom("tenant:"+tenantID.String(), "ITEM_AVAILABILITY_CHANGED", map[string]interface{}{
		"item_id":      itemID,
		"is_available": req.IsAvailable,
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"item_id":      itemID,
		"is_available": req.IsAvailable,
	})
}

// ---------------------------------------------------------------------
// Category Management (Admin CRUD)
// ---------------------------------------------------------------------

type CategoryRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	DisplayOrder int    `json:"display_order"`
	IsActive     bool   `json:"is_active"`
}

func (h *MenuHandler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	var req CategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		errors.WriteError(w, errors.BadRequest("Category name is required"))
		return
	}

	id := uuid.New()
	isActive := true
	if r.Method == http.MethodPut {
		isActive = req.IsActive
	}

	_, err := h.pool.Exec(r.Context(), `
		INSERT INTO restoapp.menu_categories (id, tenant_id, name, description, display_order, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, tenantID, req.Name, req.Description, req.DisplayOrder, isActive)
	if err != nil {
		errors.WriteError(w, errors.Internal("Failed to create menu category"))
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":            id,
		"name":          req.Name,
		"description":   req.Description,
		"display_order": req.DisplayOrder,
		"is_active":     isActive,
	})
}

func (h *MenuHandler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	catID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid category ID format"))
		return
	}

	var req CategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		errors.WriteError(w, errors.BadRequest("Category name is required"))
		return
	}

	res, err := h.pool.Exec(r.Context(), `
		UPDATE restoapp.menu_categories
		SET name = $1, description = $2, display_order = $3, is_active = $4
		WHERE id = $5 AND tenant_id = $6
	`, req.Name, req.Description, req.DisplayOrder, req.IsActive, catID, tenantID)
	if err != nil || res.RowsAffected() == 0 {
		errors.WriteError(w, errors.NotFound("Category not found for this tenant"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":            catID,
		"name":          req.Name,
		"description":   req.Description,
		"display_order": req.DisplayOrder,
		"is_active":     req.IsActive,
	})
}

func (h *MenuHandler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	catID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid category ID format"))
		return
	}

	res, err := h.pool.Exec(r.Context(), `
		DELETE FROM restoapp.menu_categories
		WHERE id = $1 AND tenant_id = $2
	`, catID, tenantID)
	if err != nil || res.RowsAffected() == 0 {
		errors.WriteError(w, errors.NotFound("Category not found or has dependent items"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"id":      catID,
	})
}

// ---------------------------------------------------------------------
// Menu Item Management (Admin CRUD)
// ---------------------------------------------------------------------

type MenuItemRequest struct {
	CategoryID          uuid.UUID                  `json:"category_id"`
	Name                string                     `json:"name"`
	Description         string                     `json:"description"`
	Price               float64                    `json:"price"`
	FoodType            string                     `json:"food_type"` // VEG, NON_VEG, EGG, VEGAN
	ImageURL            string                     `json:"image_url"`
	IsAvailable         bool                       `json:"is_available"`
	KitchenStation      string                     `json:"kitchen_station"`
	AssignedStations    []string                   `json:"assigned_stations"`
	Allergens           []string                   `json:"allergens"`
	PreparationTimeMins int                        `json:"preparation_time_mins"`
	Variants            []models.ItemVariant       `json:"variants"`
	Modifiers           []models.ItemModifierGroup `json:"modifiers"`
}

func (h *MenuHandler) CreateMenuItem(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	var req MenuItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.CategoryID == uuid.Nil {
		errors.WriteError(w, errors.BadRequest("Item name and category_id are required"))
		return
	}

	if req.KitchenStation == "" {
		req.KitchenStation = "MAIN_KITCHEN"
	}
	if len(req.AssignedStations) == 0 {
		req.AssignedStations = []string{req.KitchenStation}
	}
	if req.FoodType == "" {
		req.FoodType = "VEG"
	}
	if req.PreparationTimeMins <= 0 {
		req.PreparationTimeMins = 15
	}
	if req.Allergens == nil {
		req.Allergens = []string{}
	}

	variantsJSON, _ := json.Marshal(req.Variants)
	modifiersJSON, _ := json.Marshal(req.Modifiers)

	id := uuid.New()
	query := `
		INSERT INTO restoapp.menu_items (
			id, tenant_id, category_id, name, description, price, food_type, image_url,
			is_available, kitchen_station, assigned_stations, allergens, preparation_time_mins,
			variants, modifiers, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW())
	`
	_, err := h.pool.Exec(r.Context(), query,
		id, tenantID, req.CategoryID, req.Name, req.Description, req.Price, req.FoodType, req.ImageURL,
		req.IsAvailable, req.KitchenStation, req.AssignedStations, req.Allergens, req.PreparationTimeMins,
		variantsJSON, modifiersJSON,
	)
	if err != nil {
		errors.WriteError(w, errors.Internal(fmt.Sprintf("Failed to create menu item: %v", err)))
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"id":      id,
		"item":    req,
	})
}

func (h *MenuHandler) UpdateMenuItem(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	itemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid item ID format"))
		return
	}

	var req MenuItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		errors.WriteError(w, errors.BadRequest("Item name is required"))
		return
	}

	if req.KitchenStation == "" {
		req.KitchenStation = "MAIN_KITCHEN"
	}
	if len(req.AssignedStations) == 0 {
		req.AssignedStations = []string{req.KitchenStation}
	}
	if req.Allergens == nil {
		req.Allergens = []string{}
	}

	variantsJSON, _ := json.Marshal(req.Variants)
	modifiersJSON, _ := json.Marshal(req.Modifiers)

	query := `
		UPDATE restoapp.menu_items
		SET category_id = $1, name = $2, description = $3, price = $4, food_type = $5,
		    image_url = $6, is_available = $7, kitchen_station = $8, assigned_stations = $9,
		    allergens = $10, preparation_time_mins = $11, variants = $12, modifiers = $13
		WHERE id = $14 AND tenant_id = $15
	`
	res, err := h.pool.Exec(r.Context(), query,
		req.CategoryID, req.Name, req.Description, req.Price, req.FoodType,
		req.ImageURL, req.IsAvailable, req.KitchenStation, req.AssignedStations,
		req.Allergens, req.PreparationTimeMins, variantsJSON, modifiersJSON,
		itemID, tenantID,
	)
	if err != nil || res.RowsAffected() == 0 {
		errors.WriteError(w, errors.NotFound("Item not found for this tenant"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"id":      itemID,
		"item":    req,
	})
}

func (h *MenuHandler) DeleteMenuItem(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	itemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid item ID format"))
		return
	}

	res, err := h.pool.Exec(r.Context(), "DELETE FROM restoapp.menu_items WHERE id = $1 AND tenant_id = $2", itemID, tenantID)
	if err != nil || res.RowsAffected() == 0 {
		errors.WriteError(w, errors.NotFound("Menu item not found for this tenant"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"id":      itemID,
	})
}

// ---------------------------------------------------------------------
// Branch-Specific Pricing (Multi-branch customization)
// ---------------------------------------------------------------------

type OutletPricingRequest struct {
	Price       float64 `json:"price"`
	IsAvailable bool    `json:"is_available"`
}

func (h *MenuHandler) UpdateOutletPricing(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	itemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid item ID format"))
		return
	}

	outletID, err := uuid.Parse(chi.URLParam(r, "outlet_id"))
	if err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid outlet ID format"))
		return
	}

	var req OutletPricingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Price < 0 {
		errors.WriteError(w, errors.BadRequest("Valid price is required"))
		return
	}

	query := `
		INSERT INTO restoapp.outlet_menu_items (
			id, tenant_id, outlet_id, item_id, price, is_available, created_at, updated_at
		) VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (tenant_id, outlet_id, item_id)
		DO UPDATE SET price = EXCLUDED.price, is_available = EXCLUDED.is_available, updated_at = NOW()
	`
	_, err = h.pool.Exec(r.Context(), query, tenantID, outletID, itemID, req.Price, req.IsAvailable)
	if err != nil {
		errors.WriteError(w, errors.Internal("Failed to update branch-specific pricing"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"item_id":      itemID,
		"outlet_id":    outletID,
		"price":        req.Price,
		"is_available": req.IsAvailable,
	})
}

// ---------------------------------------------------------------------
// Bulk Import Menu Items via CSV
// ---------------------------------------------------------------------

func (h *MenuHandler) ImportCSV(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	// Read CSV content from body or multipart
	var csvReader *csv.Reader
	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		err := r.ParseMultipartForm(10 << 20) // 10MB
		if err != nil {
			errors.WriteError(w, errors.BadRequest("Failed to parse form file"))
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			errors.WriteError(w, errors.BadRequest("CSV file is required"))
			return
		}
		defer file.Close()
		csvReader = csv.NewReader(file)
	} else {
		// Read raw body as CSV text
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil || len(bodyBytes) == 0 {
			errors.WriteError(w, errors.BadRequest("Empty CSV content"))
			return
		}
		csvReader = csv.NewReader(bytes.NewReader(bodyBytes))
	}

	records, err := csvReader.ReadAll()
	if err != nil || len(records) < 2 {
		errors.WriteError(w, errors.BadRequest("Invalid CSV format or missing header/rows"))
		return
	}

	// Header mapping: category, name, description, price, food_type, preparation_time_mins, kitchen_station, allergens
	header := records[0]
	colMap := make(map[string]int)
	for i, col := range header {
		colMap[strings.TrimSpace(strings.ToLower(col))] = i
	}

	requiredCols := []string{"category", "name", "price"}
	for _, req := range requiredCols {
		if _, ok := colMap[req]; !ok {
			errors.WriteError(w, errors.BadRequest(fmt.Sprintf("Missing required column in CSV header: %s", req)))
			return
		}
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		errors.WriteError(w, errors.Internal("Failed to begin transaction"))
		return
	}
	defer tx.Rollback(ctx)

	importedCount := 0

	// Cache category IDs
	categoryCache := make(map[string]uuid.UUID)
	catRows, err := tx.Query(ctx, "SELECT id, name FROM restoapp.menu_categories WHERE tenant_id = $1", tenantID)
	if err == nil {
		defer catRows.Close()
		for catRows.Next() {
			var id uuid.UUID
			var name string
			if err := catRows.Scan(&id, &name); err == nil {
				categoryCache[strings.ToLower(strings.TrimSpace(name))] = id
			}
		}
	}

	for _, row := range records[1:] {
		if len(row) <= colMap["category"] || len(row) <= colMap["name"] || len(row) <= colMap["price"] {
			continue
		}

		catName := strings.TrimSpace(row[colMap["category"]])
		itemName := strings.TrimSpace(row[colMap["name"]])
		priceStr := strings.TrimSpace(row[colMap["price"]])

		if catName == "" || itemName == "" {
			continue
		}

		price, err := strconv.ParseFloat(priceStr, 64)
		if err != nil || price < 0 {
			continue
		}

		// Ensure category exists
		catKey := strings.ToLower(catName)
		catID, exists := categoryCache[catKey]
		if !exists {
			catID = uuid.New()
			_, err = tx.Exec(ctx, "INSERT INTO restoapp.menu_categories (id, tenant_id, name, display_order) VALUES ($1, $2, $3, 99)", catID, tenantID, catName)
			if err != nil {
				continue
			}
			categoryCache[catKey] = catID
		}

		desc := ""
		if idx, ok := colMap["description"]; ok && len(row) > idx {
			desc = strings.TrimSpace(row[idx])
		}

		foodType := "VEG"
		if idx, ok := colMap["food_type"]; ok && len(row) > idx && row[idx] != "" {
			foodType = strings.ToUpper(strings.TrimSpace(row[idx]))
		}

		prepTime := 15
		if idx, ok := colMap["preparation_time_mins"]; ok && len(row) > idx && row[idx] != "" {
			if pt, err := strconv.Atoi(strings.TrimSpace(row[idx])); err == nil {
				prepTime = pt
			}
		}

		station := "MAIN_KITCHEN"
		if idx, ok := colMap["kitchen_station"]; ok && len(row) > idx && row[idx] != "" {
			station = strings.ToUpper(strings.TrimSpace(row[idx]))
		}

		var allergens []string
		if idx, ok := colMap["allergens"]; ok && len(row) > idx && row[idx] != "" {
			for _, a := range strings.Split(row[idx], ";") {
				trimmed := strings.ToUpper(strings.TrimSpace(a))
				if trimmed != "" {
					allergens = append(allergens, trimmed)
				}
			}
		}
		if allergens == nil {
			allergens = []string{}
		}

		itemID := uuid.New()
		_, err = tx.Exec(ctx, `
			INSERT INTO restoapp.menu_items (
				id, tenant_id, category_id, name, description, price, food_type,
				kitchen_station, assigned_stations, allergens, preparation_time_mins, is_available
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, true)
		`, itemID, tenantID, catID, itemName, desc, price, foodType, station, []string{station}, allergens, prepTime)
		if err == nil {
			importedCount++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		errors.WriteError(w, errors.Internal("Failed to commit CSV bulk import"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":        true,
		"imported_count": importedCount,
		"message":        fmt.Sprintf("Successfully imported %d menu items", importedCount),
	})
}
