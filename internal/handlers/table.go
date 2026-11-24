package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rishabharidas/restolink/internal/errors"
	"github.com/rishabharidas/restolink/internal/middleware"
	"github.com/rishabharidas/restolink/internal/models"
	"github.com/rishabharidas/restolink/internal/realtime"
	"github.com/rishabharidas/restolink/internal/service"
)

type TableHandler struct {
	pool *pgxpool.Pool
	rdb  *redis.Client
	hub  *realtime.Hub
}

func NewTableHandler(pool *pgxpool.Pool, rdb *redis.Client, hub *realtime.Hub) *TableHandler {
	return &TableHandler{pool: pool, rdb: rdb, hub: hub}
}

// ---------------------------------------------------------------------
// Server Call Bell & Multiplayer Table Cart
// ---------------------------------------------------------------------

// RingBell handles diner button taps for calling waiter, water, or bill
func (h *TableHandler) RingBell(w http.ResponseWriter, r *http.Request) {
	tableIDStr := chi.URLParam(r, "id")
	tableID, err := uuid.Parse(tableIDStr)
	if err != nil {
		http.Error(w, `{"error":"bad_request","message":"invalid table id"}`, http.StatusBadRequest)
		return
	}

	var req models.TableBellRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad_request","message":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		_ = h.pool.QueryRow(r.Context(), "SELECT tenant_id, table_number FROM restoapp.restaurant_tables WHERE id = $1", tableID).
			Scan(&tenantID, &req.TableNumber)
	}

	actionLabel := "Server Assistance"
	switch req.Action {
	case models.BellCallWaiter:
		actionLabel = "Server Call"
	case models.BellRequestWater:
		actionLabel = "Water Refill Requested"
	case models.BellRequestBill:
		actionLabel = "Bill Settlement Requested"
	}

	alert := models.TableBellAlert{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		TableID:      tableID,
		TableNumber:  req.TableNumber,
		Action:       req.Action,
		ActionLabel:  actionLabel,
		CustomerName: req.CustomerName,
		Notes:        req.Notes,
		CreatedAt:    time.Now(),
	}

	if h.rdb != nil {
		alertBytes, _ := json.Marshal(alert)
		h.rdb.LPush(r.Context(), "alerts:"+tenantID.String(), alertBytes)
		h.rdb.LTrim(r.Context(), "alerts:"+tenantID.String(), 0, 49) // Keep last 50
	}

	h.hub.BroadcastToRoom("tenant:"+tenantID.String(), "TABLE_BELL_RING", alert)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"alert":   alert,
		"message": fmt.Sprintf("%s alerted for Table %s", actionLabel, req.TableNumber),
	})
}

// GetTableCart retrieves the shared multiplayer cart from Redis
func (h *TableHandler) GetTableCart(w http.ResponseWriter, r *http.Request) {
	tableIDStr := chi.URLParam(r, "id")
	tableID, err := uuid.Parse(tableIDStr)
	if err != nil {
		http.Error(w, `{"error":"bad_request","message":"invalid table id"}`, http.StatusBadRequest)
		return
	}

	var session models.TableSession
	session.TableID = tableID
	session.Items = make([]models.MultiplayerCartItem, 0)

	if h.rdb != nil {
		data, err := h.rdb.Get(r.Context(), "table_cart:"+tableID.String()).Result()
		if err == nil {
			_ = json.Unmarshal([]byte(data), &session)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(session)
}

// SyncTableCart synchronizes the shared multiplayer table cart
func (h *TableHandler) SyncTableCart(w http.ResponseWriter, r *http.Request) {
	tableIDStr := chi.URLParam(r, "id")
	tableID, err := uuid.Parse(tableIDStr)
	if err != nil {
		http.Error(w, `{"error":"bad_request","message":"invalid table id"}`, http.StatusBadRequest)
		return
	}

	var session models.TableSession
	if err := json.NewDecoder(r.Body).Decode(&session); err != nil {
		http.Error(w, `{"error":"bad_request","message":"invalid cart payload"}`, http.StatusBadRequest)
		return
	}

	session.TableID = tableID
	session.UpdatedAt = time.Now()

	if h.rdb != nil {
		data, _ := json.Marshal(session)
		h.rdb.Set(r.Context(), "table_cart:"+tableID.String(), data, 4*time.Hour)
	}

	h.hub.BroadcastToRoom("table:"+tableID.String(), "MULTIPLAYER_CART_UPDATED", session)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"session": session,
	})
}

// GenerateTableQR creates secure, cryptographic tokens for table stands
func (h *TableHandler) GenerateTableQR(w http.ResponseWriter, r *http.Request) {
	type GenQRRequest struct {
		TableNumber string `json:"table_number"`
		Capacity    int    `json:"capacity"`
	}

	var req GenQRRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad_request","message":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		http.Error(w, `{"error":"not_found","message":"tenant not found"}`, http.StatusNotFound)
		return
	}

	hasher := sha256.New()
	hasher.Write([]byte(fmt.Sprintf("%s-%s-%d", tenantID, req.TableNumber, time.Now().UnixNano())))
	token := "qr_" + hex.EncodeToString(hasher.Sum(nil))[:24]

	var outletID uuid.UUID
	_ = h.pool.QueryRow(r.Context(), "SELECT id FROM restoapp.outlets WHERE tenant_id = $1 LIMIT 1", tenantID).Scan(&outletID)

	tableID := uuid.New()
	_, err := h.pool.Exec(r.Context(), `
		INSERT INTO restoapp.restaurant_tables (id, tenant_id, outlet_id, table_number, qr_code_token, capacity)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (qr_code_token) DO UPDATE SET table_number = EXCLUDED.table_number
	`, tableID, tenantID, outletID, req.TableNumber, token, req.Capacity)
	if err != nil {
		http.Error(w, `{"error":"database_error","message":"failed to save table"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"table_id":       tableID,
		"table_number":   req.TableNumber,
		"qr_token":       token,
		"qr_landing_url": fmt.Sprintf("/table/%s", token),
	})
}

// ---------------------------------------------------------------------
// Waiter Handheld POS Table Operations
// ---------------------------------------------------------------------

type TableOverview struct {
	ID                uuid.UUID  `json:"id"`
	TenantID          uuid.UUID  `json:"tenant_id"`
	OutletID          uuid.UUID  `json:"outlet_id"`
	TableNumber       string     `json:"table_number"`
	QRCodeToken       string     `json:"qr_code_token"`
	Capacity          int        `json:"capacity"`
	Status            string     `json:"status"` // AVAILABLE, OCCUPIED, RESERVED
	ActiveOrderID     *uuid.UUID `json:"active_order_id,omitempty"`
	ActiveOrderNumber string     `json:"active_order_number,omitempty"`
	ActiveOrderStatus string     `json:"active_order_status,omitempty"`
	ActiveOrderTotal  float64    `json:"active_order_total,omitempty"`
	CustomerName      string     `json:"customer_name,omitempty"`
	ElapsedMinutes    int        `json:"elapsed_minutes,omitempty"`
}

func (h *TableHandler) ListTables(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	query := `
		SELECT t.id, t.tenant_id, t.outlet_id, t.table_number, t.qr_code_token, t.capacity, t.status,
		       t.active_order_id, COALESCE(o.order_number, ''), COALESCE(o.status::text, ''),
		       COALESCE(o.total_amount, 0), COALESCE(o.customer_name, ''),
		       COALESCE(EXTRACT(EPOCH FROM (NOW() - o.created_at)) / 60, 0)::int
		FROM restoapp.restaurant_tables t
		LEFT JOIN restoapp.orders o ON t.active_order_id = o.id
		WHERE t.tenant_id = $1
		ORDER BY t.table_number ASC
	`
	rows, err := h.pool.Query(r.Context(), query, tenantID)
	if err != nil {
		errors.WriteError(w, errors.Internal("Failed to list tables"))
		return
	}
	defer rows.Close()

	tables := make([]TableOverview, 0)
	for rows.Next() {
		var t TableOverview
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.OutletID, &t.TableNumber, &t.QRCodeToken, &t.Capacity, &t.Status,
			&t.ActiveOrderID, &t.ActiveOrderNumber, &t.ActiveOrderStatus, &t.ActiveOrderTotal,
			&t.CustomerName, &t.ElapsedMinutes,
		); err == nil {
			tables = append(tables, t)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tables)
}

// AppendItemsToOrder appends additional rounds of dishes/drinks to a table's active order
type AppendItemsRequest struct {
	Items []CreateOrderItemRequest `json:"items"`
}

func (h *TableHandler) AppendItemsToOrder(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	tableID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid table ID"))
		return
	}

	var req AppendItemsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Items) == 0 {
		errors.WriteError(w, errors.BadRequest("At least one item is required"))
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		errors.WriteError(w, errors.Internal("Failed to begin transaction"))
		return
	}
	defer tx.Rollback(ctx)

	// Fetch table's active order
	var orderID uuid.UUID
	var outletID uuid.UUID
	var currentSubtotal, currentTax, currentDelivery float64
	var orderType models.OrderType

	err = tx.QueryRow(ctx, `
		SELECT o.id, o.outlet_id, o.subtotal, o.tax_amount, o.delivery_fee, o.order_type
		FROM restoapp.orders o
		JOIN restoapp.restaurant_tables t ON t.active_order_id = o.id
		WHERE t.id = $1 AND t.tenant_id = $2 AND o.status NOT IN ('COMPLETED', 'CANCELLED')
	`, tableID, tenantID).Scan(&orderID, &outletID, &currentSubtotal, &currentTax, &currentDelivery, &orderType)
	if err != nil {
		errors.WriteError(w, errors.NotFound("No active open order found for this table"))
		return
	}

	// Calculate prices for new items
	var addedSubtotal float64
	for _, it := range req.Items {
		var price float64
		var name string
		var station string
		err := tx.QueryRow(ctx, `
			SELECT name, price, COALESCE(kitchen_station, 'MAIN_KITCHEN')
			FROM restoapp.menu_items
			WHERE id = $1 AND tenant_id = $2 AND is_available = true
		`, it.ItemID, tenantID).Scan(&name, &price, &station)
		if err != nil {
			errors.WriteError(w, errors.BadRequest(fmt.Sprintf("Item %s is invalid or unavailable", it.ItemID)))
			return
		}

		itemTotal := price * float64(it.Quantity)
		addedSubtotal += itemTotal

		assignedStation := station
		if it.AssignedStation != "" {
			assignedStation = it.AssignedStation
		}

		modifiersJSON, _ := json.Marshal(it.SelectedModifiers)
		_, err = tx.Exec(ctx, `
			INSERT INTO restoapp.order_items (
				id, order_id, item_id, quantity, unit_price, item_total,
				item_status, assigned_station, special_instructions, selected_modifiers
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, uuid.New(), orderID, it.ItemID, it.Quantity, price, itemTotal,
			models.StatusPlaced, assignedStation, it.SpecialInstructions, modifiersJSON)
		if err != nil {
			errors.WriteError(w, errors.Internal("Failed to append order items"))
			return
		}
	}

	// Recalculate bill totals
	newSubtotal := currentSubtotal + addedSubtotal
	newTax, newDelivery, newTotal := service.CalculateBill(newSubtotal, orderType, 5.0)

	_, err = tx.Exec(ctx, `
		UPDATE restoapp.orders
		SET subtotal = $1, tax_amount = $2, delivery_fee = $3, total_amount = $4, updated_at = NOW()
		WHERE id = $5
	`, newSubtotal, newTax, newDelivery, newTotal, orderID)
	if err != nil {
		errors.WriteError(w, errors.Internal("Failed to update order totals"))
		return
	}

	// Log audit trail
	_, _ = tx.Exec(ctx, `
		INSERT INTO restoapp.order_status_history (order_id, new_status, note)
		VALUES ($1, 'PLACED', 'Waiter appended additional dishes to order')
	`, orderID)

	if err := tx.Commit(ctx); err != nil {
		errors.WriteError(w, errors.Internal("Failed to commit order update"))
		return
	}

	// Broadcast event
	h.hub.BroadcastToRoom("tenant:"+tenantID.String(), "ORDER_ITEMS_APPENDED", map[string]interface{}{
		"order_id":     orderID,
		"table_id":     tableID,
		"total_amount": newTotal,
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"order_id":     orderID,
		"new_subtotal": newSubtotal,
		"new_total":    newTotal,
	})
}

// TransferTable moves an active order from one table to another
type TransferTableRequest struct {
	FromTableID uuid.UUID `json:"from_table_id"`
	ToTableID   uuid.UUID `json:"to_table_id"`
}

func (h *TableHandler) TransferTable(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	var req TransferTableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FromTableID == uuid.Nil || req.ToTableID == uuid.Nil {
		errors.WriteError(w, errors.BadRequest("Both from_table_id and to_table_id are required"))
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		errors.WriteError(w, errors.Internal("Failed to begin transaction"))
		return
	}
	defer tx.Rollback(ctx)

	// Verify from table has active order
	var orderID *uuid.UUID
	var fromTableNum string
	err = tx.QueryRow(ctx, "SELECT active_order_id, table_number FROM restoapp.restaurant_tables WHERE id = $1 AND tenant_id = $2", req.FromTableID, tenantID).
		Scan(&orderID, &fromTableNum)
	if err != nil || orderID == nil {
		errors.WriteError(w, errors.BadRequest("Source table has no active order"))
		return
	}

	// Verify target table is available
	var toTableStatus string
	var toTableNum string
	err = tx.QueryRow(ctx, "SELECT status, table_number FROM restoapp.restaurant_tables WHERE id = $1 AND tenant_id = $2", req.ToTableID, tenantID).
		Scan(&toTableStatus, &toTableNum)
	if err != nil || toTableStatus == "OCCUPIED" {
		errors.WriteError(w, errors.BadRequest("Destination table is occupied or invalid"))
		return
	}

	// Move order
	_, err = tx.Exec(ctx, "UPDATE restoapp.orders SET table_id = $1, updated_at = NOW() WHERE id = $2", req.ToTableID, orderID)
	if err != nil {
		errors.WriteError(w, errors.Internal("Failed to update order table"))
		return
	}

	// Reset source table
	_, _ = tx.Exec(ctx, "UPDATE restoapp.restaurant_tables SET status = 'AVAILABLE', active_order_id = NULL WHERE id = $1", req.FromTableID)

	// Set target table
	_, _ = tx.Exec(ctx, "UPDATE restoapp.restaurant_tables SET status = 'OCCUPIED', active_order_id = $1 WHERE id = $2", orderID, req.ToTableID)

	// Audit history
	_, _ = tx.Exec(ctx, `
		INSERT INTO restoapp.order_status_history (order_id, new_status, note)
		VALUES ($1, 'PLACED', $2)
	`, orderID, fmt.Sprintf("Table transferred from %s to %s", fromTableNum, toTableNum))

	if err := tx.Commit(ctx); err != nil {
		errors.WriteError(w, errors.Internal("Failed to commit transfer transaction"))
		return
	}

	h.hub.BroadcastToRoom("tenant:"+tenantID.String(), "TABLE_TRANSFERRED", map[string]interface{}{
		"order_id":        orderID,
		"from_table_id":   req.FromTableID,
		"to_table_id":     req.ToTableID,
		"from_table_num":  fromTableNum,
		"to_table_num":    toTableNum,
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":        true,
		"message":        fmt.Sprintf("Transferred order from Table %s to Table %s", fromTableNum, toTableNum),
		"from_table_id":  req.FromTableID,
		"to_table_id":    req.ToTableID,
	})
}

// MergeTables merges multiple occupied source tables into one target table
type MergeTablesRequest struct {
	SourceTableIDs []uuid.UUID `json:"source_table_ids"`
	TargetTableID  uuid.UUID   `json:"target_table_id"`
}

func (h *TableHandler) MergeTables(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	var req MergeTablesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.SourceTableIDs) == 0 || req.TargetTableID == uuid.Nil {
		errors.WriteError(w, errors.BadRequest("Valid source_table_ids and target_table_id are required"))
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		errors.WriteError(w, errors.Internal("Failed to begin transaction"))
		return
	}
	defer tx.Rollback(ctx)

	// Fetch target table's active order
	var targetOrderID *uuid.UUID
	var targetTableNum string
	err = tx.QueryRow(ctx, "SELECT active_order_id, table_number FROM restoapp.restaurant_tables WHERE id = $1 AND tenant_id = $2", req.TargetTableID, tenantID).
		Scan(&targetOrderID, &targetTableNum)
	if err != nil || targetOrderID == nil {
		errors.WriteError(w, errors.BadRequest("Target table must have an active open order"))
		return
	}

	// For each source table, reassign its order items into target order and free source table
	for _, sourceTableID := range req.SourceTableIDs {
		if sourceTableID == req.TargetTableID {
			continue
		}

		var sourceOrderID *uuid.UUID
		err := tx.QueryRow(ctx, "SELECT active_order_id FROM restoapp.restaurant_tables WHERE id = $1 AND tenant_id = $2", sourceTableID, tenantID).
			Scan(&sourceOrderID)
		if err == nil && sourceOrderID != nil {
			// Move items
			_, _ = tx.Exec(ctx, "UPDATE restoapp.order_items SET order_id = $1 WHERE order_id = $2", targetOrderID, sourceOrderID)
			// Cancel source order as MERGED
			_, _ = tx.Exec(ctx, "UPDATE restoapp.orders SET status = 'CANCELLED', updated_at = NOW() WHERE id = $1", sourceOrderID)
			_, _ = tx.Exec(ctx, `
				INSERT INTO restoapp.order_status_history (order_id, previous_status, new_status, note)
				VALUES ($1, 'PLACED', 'CANCELLED', 'Merged into Table ' || $2)
			`, sourceOrderID, targetTableNum)
		}

		// Free source table
		_, _ = tx.Exec(ctx, "UPDATE restoapp.restaurant_tables SET status = 'AVAILABLE', active_order_id = NULL WHERE id = $1", sourceTableID)
	}

	// Recalculate target order totals
	var newSubtotal float64
	_ = tx.QueryRow(ctx, "SELECT COALESCE(SUM(item_total), 0) FROM restoapp.order_items WHERE order_id = $1", targetOrderID).Scan(&newSubtotal)
	newTax, newDelivery, newTotal := service.CalculateBill(newSubtotal, models.OrderTypeDineIn, 5.0)

	_, _ = tx.Exec(ctx, `
		UPDATE restoapp.orders
		SET subtotal = $1, tax_amount = $2, delivery_fee = $3, total_amount = $4, updated_at = NOW()
		WHERE id = $5
	`, newSubtotal, newTax, newDelivery, newTotal, targetOrderID)

	if err := tx.Commit(ctx); err != nil {
		errors.WriteError(w, errors.Internal("Failed to commit merge transaction"))
		return
	}

	h.hub.BroadcastToRoom("tenant:"+tenantID.String(), "TABLES_MERGED", map[string]interface{}{
		"target_table_id":   req.TargetTableID,
		"source_table_ids":  req.SourceTableIDs,
		"new_total":         newTotal,
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":         true,
		"target_table_id": req.TargetTableID,
		"new_total":       newTotal,
	})
}
