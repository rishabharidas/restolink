package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rishabharidas/restolink/internal/errors"
	"github.com/rishabharidas/restolink/internal/middleware"
	"github.com/rishabharidas/restolink/internal/models"
	"github.com/rishabharidas/restolink/internal/realtime"
	"github.com/rishabharidas/restolink/internal/service"
)

type OrderHandler struct {
	pool *pgxpool.Pool
	hub  *realtime.Hub
}

func NewOrderHandler(pool *pgxpool.Pool, hub *realtime.Hub) *OrderHandler {
	return &OrderHandler{pool: pool, hub: hub}
}

type CreateOrderItemRequest struct {
	ItemID              uuid.UUID                   `json:"item_id"`
	Quantity            int                         `json:"quantity"`
	VariantID           string                      `json:"variant_id,omitempty"`
	SelectedModifiers   []models.ItemModifierOption `json:"selected_modifiers,omitempty"`
	SpecialInstructions string                      `json:"special_instructions,omitempty"`
	AssignedStation     string                      `json:"assigned_station,omitempty"`
}

type CreateOrderRequest struct {
	IdempotencyKey      string                   `json:"idempotency_key,omitempty"`
	OrderType           models.OrderType         `json:"order_type"` // DINE_IN, DELIVERY, TAKEAWAY
	TableID             *uuid.UUID               `json:"table_id,omitempty"`
	OutletID            *uuid.UUID               `json:"outlet_id,omitempty"`
	CustomerName        string                   `json:"customer_name"`
	CustomerPhone       string                   `json:"customer_phone"`
	DeliveryAddress     string                   `json:"delivery_address,omitempty"`
	DeliveryLat         float64                  `json:"delivery_lat,omitempty"`
	DeliveryLng         float64                  `json:"delivery_lng,omitempty"`
	SpecialInstructions string                   `json:"special_instructions,omitempty"`
	Items               []CreateOrderItemRequest `json:"items"`
}

func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	var req CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid request body"))
		return
	}

	if len(req.Items) == 0 {
		errors.WriteError(w, errors.BadRequest("Order must contain at least one item"))
		return
	}

	ctx := r.Context()

	// 1. Idempotency Check: Return existing order if key is present and matched
	if req.IdempotencyKey != "" {
		var existingID uuid.UUID
		var existingNumber string
		var existingTotal float64
		var existingStatus models.OrderStatus
		err := h.pool.QueryRow(ctx, `
			SELECT id, order_number, total_amount, status
			FROM restoapp.orders
			WHERE tenant_id = $1 AND idempotency_key = $2
		`, tenantID, req.IdempotencyKey).Scan(&existingID, &existingNumber, &existingTotal, &existingStatus)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success":      true,
				"idempotent":   true,
				"order_id":     existingID,
				"order_number": existingNumber,
				"total_amount": existingTotal,
				"status":       existingStatus,
				"tracking_url": fmt.Sprintf("/track/%s", existingID),
			})
			return
		}
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		errors.WriteError(w, errors.Internal("Failed to begin transaction"))
		return
	}
	defer tx.Rollback(ctx)

	// Fetch tenant default outlet and tax rate
	var outletID uuid.UUID
	if req.OutletID != nil && *req.OutletID != uuid.Nil {
		outletID = *req.OutletID
	} else {
		err = tx.QueryRow(ctx, "SELECT id FROM restoapp.outlets WHERE tenant_id = $1 LIMIT 1", tenantID).Scan(&outletID)
		if err != nil {
			errors.WriteError(w, errors.Internal("Outlet not configured for tenant"))
			return
		}
	}

	var gstPercentage float64 = 5.0
	var settingsRaw []byte
	if err := tx.QueryRow(ctx, "SELECT settings FROM restoapp.tenants WHERE id = $1", tenantID).Scan(&settingsRaw); err == nil {
		var s models.TenantSettings
		if err := json.Unmarshal(settingsRaw, &s); err == nil && s.TaxGSTPercentage > 0 {
			gstPercentage = s.TaxGSTPercentage
		}
	}

	// 2. Strict Backend Price Calculation & Item Validation
	var subtotal float64
	type itemCalculated struct {
		ItemID            uuid.UUID
		Name              string
		UnitPrice         float64
		Quantity          int
		ItemTotal         float64
		AssignedStation   string
		Notes             string
		SelectedModifiers []models.ItemModifierOption
	}
	var calculatedItems []itemCalculated

	for _, reqItem := range req.Items {
		var basePrice float64
		var name string
		var station string
		var variantsRaw []byte
		err := tx.QueryRow(ctx, `
			SELECT name, price, COALESCE(kitchen_station, 'MAIN_KITCHEN'), COALESCE(variants, '[]'::jsonb)
			FROM restoapp.menu_items
			WHERE id = $1 AND tenant_id = $2 AND is_available = true
		`, reqItem.ItemID, tenantID).Scan(&name, &basePrice, &station, &variantsRaw)
		if err != nil {
			errors.WriteError(w, errors.BadRequest(fmt.Sprintf("Item %s is invalid, unavailable, or does not belong to this restaurant", reqItem.ItemID)))
			return
		}

		// Check branch-specific price override
		var outletPrice float64
		err = tx.QueryRow(ctx, `
			SELECT price FROM restoapp.outlet_menu_items
			WHERE tenant_id = $1 AND outlet_id = $2 AND item_id = $3 AND is_available = true
		`, tenantID, outletID, reqItem.ItemID).Scan(&outletPrice)
		if err == nil && outletPrice > 0 {
			basePrice = outletPrice
		}

		// Calculate variant adjustment
		variantAdj := 0.0
		if reqItem.VariantID != "" && len(variantsRaw) > 0 {
			var variants []models.ItemVariant
			if err := json.Unmarshal(variantsRaw, &variants); err == nil {
				for _, v := range variants {
					if v.ID == reqItem.VariantID {
						variantAdj = v.PriceAdjustment
						break
					}
				}
			}
		}

		// Calculate modifiers sum
		modTotal := 0.0
		for _, mod := range reqItem.SelectedModifiers {
			modTotal += mod.PriceDiff
		}

		effectiveUnitPrice := basePrice + variantAdj + modTotal
		if effectiveUnitPrice < 0 {
			effectiveUnitPrice = 0
		}
		itemTotal := effectiveUnitPrice * float64(reqItem.Quantity)
		subtotal += itemTotal

		assignedStation := station
		if reqItem.AssignedStation != "" {
			assignedStation = reqItem.AssignedStation
		}

		calculatedItems = append(calculatedItems, itemCalculated{
			ItemID:            reqItem.ItemID,
			Name:              name,
			UnitPrice:         effectiveUnitPrice,
			Quantity:          reqItem.Quantity,
			ItemTotal:         itemTotal,
			AssignedStation:   assignedStation,
			Notes:             reqItem.SpecialInstructions,
			SelectedModifiers: reqItem.SelectedModifiers,
		})
	}

	taxAmount, deliveryFee, totalAmount := service.CalculateBill(subtotal, req.OrderType, gstPercentage)
	orderNumber := fmt.Sprintf("ORD-%d", time.Now().Unix()%100000)
	orderID := uuid.New()

	var idempotencyPtr *string
	if req.IdempotencyKey != "" {
		idempotencyPtr = &req.IdempotencyKey
	}

	// 3. Insert order
	insertOrderQuery := `
		INSERT INTO restoapp.orders (
			id, tenant_id, outlet_id, order_number, order_type, status,
			table_id, customer_name, customer_phone, special_instructions,
			subtotal, tax_amount, delivery_fee, total_amount, payment_status,
			idempotency_key
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, 'PENDING', $15)
	`
	_, err = tx.Exec(ctx, insertOrderQuery,
		orderID, tenantID, outletID, orderNumber, req.OrderType, models.StatusPlaced,
		req.TableID, req.CustomerName, req.CustomerPhone, req.SpecialInstructions,
		subtotal, taxAmount, deliveryFee, totalAmount, idempotencyPtr,
	)
	if err != nil {
		errors.WriteError(w, errors.Internal(fmt.Sprintf("Failed to insert order: %v", err)))
		return
	}

	// If table order, associate table's active_order_id and set OCCUPIED
	if req.TableID != nil && *req.TableID != uuid.Nil {
		_, _ = tx.Exec(ctx, `
			UPDATE restoapp.restaurant_tables 
			SET status = 'OCCUPIED', active_order_id = $1 
			WHERE id = $2 AND tenant_id = $3
		`, orderID, req.TableID, tenantID)
	}

	// 4. Insert order items
	for _, ci := range calculatedItems {
		modifiersJSON, _ := json.Marshal(ci.SelectedModifiers)
		_, err := tx.Exec(ctx, `
			INSERT INTO restoapp.order_items (
				id, order_id, item_id, quantity, unit_price, item_total,
				item_status, assigned_station, special_instructions, selected_modifiers
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, uuid.New(), orderID, ci.ItemID, ci.Quantity, ci.UnitPrice, ci.ItemTotal,
			models.StatusPlaced, ci.AssignedStation, ci.Notes, modifiersJSON)
		if err != nil {
			errors.WriteError(w, errors.Internal("Failed to insert order items"))
			return
		}
	}

	// 5. Insert initial status history audit trail
	_, _ = tx.Exec(ctx, `
		INSERT INTO restoapp.order_status_history (order_id, new_status, note)
		VALUES ($1, $2, 'Order placed successfully')
	`, orderID, models.StatusPlaced)

	// If delivery, create delivery record
	if req.OrderType == models.OrderTypeDelivery {
		otp := fmt.Sprintf("%04d", time.Now().UnixNano()%10000)
		_, _ = tx.Exec(ctx, `
			INSERT INTO restoapp.deliveries (id, tenant_id, order_id, delivery_address, delivery_lat, delivery_lng, customer_phone, delivery_otp)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, uuid.New(), tenantID, orderID, req.DeliveryAddress, req.DeliveryLat, req.DeliveryLng, req.CustomerPhone, otp)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		errors.WriteError(w, errors.Internal("Failed to commit order transaction"))
		return
	}

	// 6. Broadcast NEW_ORDER event to tenant room (Kitchen KDS + Waiters)
	h.hub.BroadcastToRoom("tenant:"+tenantID.String(), "NEW_ORDER", map[string]interface{}{
		"order_id":     orderID,
		"order_number": orderNumber,
		"order_type":   req.OrderType,
		"total_amount": totalAmount,
		"status":       models.StatusPlaced,
		"table_id":     req.TableID,
	})

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"order_id":     orderID,
		"order_number": orderNumber,
		"order_type":   req.OrderType,
		"subtotal":     subtotal,
		"tax_amount":   taxAmount,
		"total_amount": totalAmount,
		"status":       models.StatusPlaced,
		"tracking_url": fmt.Sprintf("/track/%s", orderID),
	})
}

// ---------------------------------------------------------------------
// Order Retrieval & List
// ---------------------------------------------------------------------

func (h *OrderHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	activeTenantID := middleware.GetTenantID(r.Context())
	if activeTenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	orderIDStr := chi.URLParam(r, "id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid order ID format"))
		return
	}

	var ord models.Order
	var tableNumber *string
	query := `
		SELECT o.id, o.tenant_id, o.outlet_id, o.order_number, o.order_type, o.status,
		       o.table_id, t.table_number,
		       COALESCE(o.customer_name, ''), COALESCE(o.customer_phone, ''),
		       o.subtotal, o.tax_amount, o.delivery_fee, o.total_amount, o.payment_status,
		       o.created_at, o.updated_at
		FROM restoapp.orders o
		LEFT JOIN restoapp.restaurant_tables t ON o.table_id = t.id
		WHERE o.id = $1 AND o.tenant_id = $2
	`
	err = h.pool.QueryRow(r.Context(), query, orderID, activeTenantID).Scan(
		&ord.ID, &ord.TenantID, &ord.OutletID, &ord.OrderNumber, &ord.OrderType, &ord.Status,
		&ord.TableID, &tableNumber,
		&ord.CustomerName, &ord.CustomerPhone,
		&ord.Subtotal, &ord.TaxAmount, &ord.DeliveryFee, &ord.TotalAmount, &ord.PaymentStatus,
		&ord.CreatedAt, &ord.UpdatedAt,
	)
	if err != nil {
		errors.WriteError(w, errors.NotFound("Order not found"))
		return
	}
	if tableNumber != nil {
		ord.TableNumber = *tableNumber
	}

	// Fetch items
	rows, err := h.pool.Query(r.Context(), `
		SELECT oi.id, oi.item_id, mi.name, oi.quantity, oi.unit_price, oi.item_total,
		       oi.item_status, COALESCE(oi.assigned_station, 'MAIN_KITCHEN'),
		       COALESCE(oi.special_instructions, ''), COALESCE(oi.kitchen_notes, ''),
		       oi.completed_at, COALESCE(oi.selected_modifiers, '[]'::jsonb), oi.created_at
		FROM restoapp.order_items oi
		JOIN restoapp.menu_items mi ON oi.item_id = mi.id
		WHERE oi.order_id = $1
	`, orderID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var it models.OrderItem
			var modRaw []byte
			if err := rows.Scan(
				&it.ID, &it.ItemID, &it.ItemName, &it.Quantity, &it.UnitPrice, &it.ItemTotal,
				&it.ItemStatus, &it.AssignedStation, &it.SpecialInstructions, &it.KitchenNotes,
				&it.CompletedAt, &modRaw, &it.CreatedAt,
			); err == nil {
				it.OrderID = orderID
				if len(modRaw) > 0 {
					_ = json.Unmarshal(modRaw, &it.SelectedModifiers)
				}
				ord.Items = append(ord.Items, it)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ord)
}

func (h *OrderHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	activeTenantID := middleware.GetTenantID(r.Context())
	if activeTenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	activeOnly := r.URL.Query().Get("active") == "true"
	stationFilter := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("station")))
	orderTypeFilter := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("type")))

	query := `
		SELECT o.id, o.tenant_id, o.outlet_id, o.order_number, o.order_type, o.status,
		       o.table_id, t.table_number,
		       COALESCE(o.customer_name, ''), COALESCE(o.customer_phone, ''),
		       o.subtotal, o.tax_amount, o.delivery_fee, o.total_amount, o.payment_status,
		       o.created_at, o.updated_at
		FROM restoapp.orders o
		LEFT JOIN restoapp.restaurant_tables t ON o.table_id = t.id
		WHERE o.tenant_id = $1
	`
	var args []interface{}
	args = append(args, activeTenantID)

	if activeOnly {
		query += " AND o.status NOT IN ('COMPLETED', 'CANCELLED', 'DELIVERED')"
	}
	if orderTypeFilter != "" {
		args = append(args, orderTypeFilter)
		query += fmt.Sprintf(" AND o.order_type = $%d", len(args))
	}

	query += " ORDER BY o.created_at DESC LIMIT 100"

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		errors.WriteError(w, errors.Internal("Failed to list orders"))
		return
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var ord models.Order
		var tableNumber *string
		if err := rows.Scan(
			&ord.ID, &ord.TenantID, &ord.OutletID, &ord.OrderNumber, &ord.OrderType, &ord.Status,
			&ord.TableID, &tableNumber,
			&ord.CustomerName, &ord.CustomerPhone,
			&ord.Subtotal, &ord.TaxAmount, &ord.DeliveryFee, &ord.TotalAmount, &ord.PaymentStatus,
			&ord.CreatedAt, &ord.UpdatedAt,
		); err != nil {
			continue
		}
		if tableNumber != nil {
			ord.TableNumber = *tableNumber
		}
		orders = append(orders, ord)
	}

	// Populate order items
	for i := range orders {
		itemRows, err := h.pool.Query(r.Context(), `
			SELECT oi.id, oi.item_id, mi.name, oi.quantity, oi.unit_price, oi.item_total,
			       oi.item_status, COALESCE(oi.assigned_station, 'MAIN_KITCHEN'),
			       COALESCE(oi.special_instructions, ''), COALESCE(oi.kitchen_notes, ''),
			       oi.completed_at, COALESCE(oi.selected_modifiers, '[]'::jsonb), oi.created_at
			FROM restoapp.order_items oi
			JOIN restoapp.menu_items mi ON oi.item_id = mi.id
			WHERE oi.order_id = $1
		`, orders[i].ID)
		if err == nil {
			for itemRows.Next() {
				var it models.OrderItem
				var modRaw []byte
				if err := itemRows.Scan(
					&it.ID, &it.ItemID, &it.ItemName, &it.Quantity, &it.UnitPrice, &it.ItemTotal,
					&it.ItemStatus, &it.AssignedStation, &it.SpecialInstructions, &it.KitchenNotes,
					&it.CompletedAt, &modRaw, &it.CreatedAt,
				); err == nil {
					it.OrderID = orders[i].ID
					if len(modRaw) > 0 {
						_ = json.Unmarshal(modRaw, &it.SelectedModifiers)
					}

					// Station filter if requested
					if stationFilter == "" || stationFilter == "ALL" || it.AssignedStation == stationFilter {
						orders[i].Items = append(orders[i].Items, it)
					}
				}
			}
			itemRows.Close()
		}
	}

	// If station filter was applied, filter out orders with 0 matching items
	if stationFilter != "" && stationFilter != "ALL" {
		var filtered []models.Order
		for _, ord := range orders {
			if len(ord.Items) > 0 {
				filtered = append(filtered, ord)
			}
		}
		orders = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(orders)
}

// ---------------------------------------------------------------------
// Order State Transitions & Cancellation
// ---------------------------------------------------------------------

type UpdateStatusRequest struct {
	Status models.OrderStatus `json:"status"`
	Note   string             `json:"note,omitempty"`
}

func (h *OrderHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	activeTenantID := middleware.GetTenantID(r.Context())
	if activeTenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	orderIDStr := chi.URLParam(r, "id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid order ID format"))
		return
	}

	var req UpdateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid request body"))
		return
	}

	ctx := r.Context()

	// 1. Fetch current order status strictly scoped to tenant
	var currentStatus models.OrderStatus
	var tenantID uuid.UUID
	var tableID *uuid.UUID
	err = h.pool.QueryRow(ctx, "SELECT status, tenant_id, table_id FROM restoapp.orders WHERE id = $1 AND tenant_id = $2", orderID, activeTenantID).
		Scan(&currentStatus, &tenantID, &tableID)
	if err != nil {
		errors.WriteError(w, errors.NotFound("Order not found for this tenant"))
		return
	}

	// 2. Validate transition against Order State Machine rules
	if err := service.ValidateOrderTransition(currentStatus, req.Status); err != nil {
		errors.WriteError(w, err)
		return
	}

	// 3. Update order status
	_, err = h.pool.Exec(ctx, "UPDATE restoapp.orders SET status = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3", req.Status, orderID, activeTenantID)
	if err != nil {
		errors.WriteError(w, errors.Internal("Failed to update order status"))
		return
	}

	// If order is completed or cancelled, release table if occupied
	if (req.Status == models.StatusCompleted || req.Status == models.StatusCancelled) && tableID != nil {
		_, _ = h.pool.Exec(ctx, "UPDATE restoapp.restaurant_tables SET status = 'AVAILABLE', active_order_id = NULL WHERE id = $1", tableID)
	}

	// 4. Append to status history audit trail
	var changedByID *uuid.UUID
	if user := middleware.GetAuthUser(ctx); user != nil {
		changedByID = &user.UserID
	}
	_, _ = h.pool.Exec(ctx, `
		INSERT INTO restoapp.order_status_history (order_id, previous_status, new_status, changed_by_user_id, note)
		VALUES ($1, $2, $3, $4, $5)
	`, orderID, currentStatus, req.Status, changedByID, req.Note)

	// 5. Broadcast real-time events
	eventPayload := map[string]interface{}{
		"order_id":        orderID,
		"previous_status": currentStatus,
		"status":          req.Status,
		"note":            req.Note,
		"updated_at":      time.Now(),
	}

	h.hub.BroadcastToRoom("order:"+orderID.String(), "ORDER_STATUS_UPDATED", eventPayload)
	h.hub.BroadcastToRoom("tenant:"+tenantID.String(), "ORDER_STATUS_UPDATED", eventPayload)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"status":  req.Status,
	})
}

type CancelOrderRequest struct {
	Reason string `json:"reason"`
}

func (h *OrderHandler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	activeTenantID := middleware.GetTenantID(r.Context())
	if activeTenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	orderIDStr := chi.URLParam(r, "id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid order ID format"))
		return
	}

	var req CancelOrderRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Reason == "" {
		req.Reason = "Order cancelled by request"
	}

	ctx := r.Context()
	var currentStatus models.OrderStatus
	var tableID *uuid.UUID
	err = h.pool.QueryRow(ctx, "SELECT status, table_id FROM restoapp.orders WHERE id = $1 AND tenant_id = $2", orderID, activeTenantID).
		Scan(&currentStatus, &tableID)
	if err != nil {
		errors.WriteError(w, errors.NotFound("Order not found"))
		return
	}

	// Cancellation rules: cannot cancel if already served, completed, or out for delivery
	if currentStatus == models.StatusServed || currentStatus == models.StatusDelivered ||
		currentStatus == models.StatusOutForDelivery || currentStatus == models.StatusCompleted {
		errors.WriteError(w, errors.BadRequest(fmt.Sprintf("Cannot cancel order in status %s", currentStatus)))
		return
	}

	_, err = h.pool.Exec(ctx, "UPDATE restoapp.orders SET status = 'CANCELLED', updated_at = NOW() WHERE id = $1 AND tenant_id = $2", orderID, activeTenantID)
	if err != nil {
		errors.WriteError(w, errors.Internal("Failed to cancel order"))
		return
	}

	// Release table if occupied
	if tableID != nil {
		_, _ = h.pool.Exec(ctx, "UPDATE restoapp.restaurant_tables SET status = 'AVAILABLE', active_order_id = NULL WHERE id = $1", tableID)
	}

	// Audit log
	_, _ = h.pool.Exec(ctx, `
		INSERT INTO restoapp.order_status_history (order_id, previous_status, new_status, note)
		VALUES ($1, $2, 'CANCELLED', $3)
	`, orderID, currentStatus, req.Reason)

	eventPayload := map[string]interface{}{
		"order_id": orderID,
		"status":   models.StatusCancelled,
		"reason":   req.Reason,
	}
	h.hub.BroadcastToRoom("order:"+orderID.String(), "ORDER_CANCELLED", eventPayload)
	h.hub.BroadcastToRoom("tenant:"+activeTenantID.String(), "ORDER_CANCELLED", eventPayload)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"status":  models.StatusCancelled,
		"reason":  req.Reason,
	})
}

// ---------------------------------------------------------------------
// Phase 4: KDS Item-Level Status Management
// ---------------------------------------------------------------------

type UpdateOrderItemStatusRequest struct {
	Status       models.OrderStatus `json:"status"`
	KitchenNotes string             `json:"kitchen_notes,omitempty"`
}

func (h *OrderHandler) UpdateOrderItemStatus(w http.ResponseWriter, r *http.Request) {
	activeTenantID := middleware.GetTenantID(r.Context())
	if activeTenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	orderID, err := uuid.Parse(chi.URLParam(r, "order_id"))
	if err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid order ID"))
		return
	}

	itemID, err := uuid.Parse(chi.URLParam(r, "item_id"))
	if err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid item ID"))
		return
	}

	var req UpdateOrderItemStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid request body"))
		return
	}

	ctx := r.Context()

	// Verify order belongs to active tenant
	var currentOrderStatus models.OrderStatus
	err = h.pool.QueryRow(ctx, "SELECT status FROM restoapp.orders WHERE id = $1 AND tenant_id = $2", orderID, activeTenantID).Scan(&currentOrderStatus)
	if err != nil {
		errors.WriteError(w, errors.NotFound("Order not found for this tenant"))
		return
	}

	var completedAt *time.Time
	if req.Status == models.StatusReadyToServe || req.Status == models.StatusServed || req.Status == models.StatusCompleted {
		now := time.Now()
		completedAt = &now
	}

	// Update order item status
	res, err := h.pool.Exec(ctx, `
		UPDATE restoapp.order_items
		SET item_status = $1, kitchen_notes = COALESCE(NULLIF($2, ''), kitchen_notes), completed_at = $3
		WHERE id = $4 AND order_id = $5
	`, req.Status, req.KitchenNotes, completedAt, itemID, orderID)
	if err != nil || res.RowsAffected() == 0 {
		errors.WriteError(w, errors.NotFound("Order item not found"))
		return
	}

	// Check if ALL items in this order are now ready
	var remainingCount int
	err = h.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM restoapp.order_items
		WHERE order_id = $1 AND item_status NOT IN ('READY_TO_SERVE', 'SERVED', 'COMPLETED', 'CANCELLED')
	`, orderID).Scan(&remainingCount)

	orderAutoAdvanced := false
	if err == nil && remainingCount == 0 && currentOrderStatus != models.StatusReadyToServe &&
		currentOrderStatus != models.StatusServed && currentOrderStatus != models.StatusCompleted {
		// Auto-advance parent order to READY_TO_SERVE!
		_, _ = h.pool.Exec(ctx, "UPDATE restoapp.orders SET status = 'READY_TO_SERVE', updated_at = NOW() WHERE id = $1", orderID)
		_, _ = h.pool.Exec(ctx, `
			INSERT INTO restoapp.order_status_history (order_id, previous_status, new_status, note)
			VALUES ($1, $2, 'READY_TO_SERVE', 'All kitchen items prepared and ready')
		`, orderID, currentOrderStatus)
		orderAutoAdvanced = true

		h.hub.BroadcastToRoom("tenant:"+activeTenantID.String(), "ORDER_STATUS_UPDATED", map[string]interface{}{
			"order_id":        orderID,
			"previous_status": currentOrderStatus,
			"status":          models.StatusReadyToServe,
			"note":            "All dishes ready to serve",
		})
	}

	// Broadcast item status update
	itemUpdatePayload := map[string]interface{}{
		"order_id":            orderID,
		"item_id":             itemID,
		"item_status":         req.Status,
		"kitchen_notes":       req.KitchenNotes,
		"order_auto_advanced": orderAutoAdvanced,
	}
	h.hub.BroadcastToRoom("tenant:"+activeTenantID.String(), "ORDER_ITEM_STATUS_UPDATED", itemUpdatePayload)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":             true,
		"order_id":            orderID,
		"item_id":             itemID,
		"item_status":         req.Status,
		"order_auto_advanced": orderAutoAdvanced,
	})
}
