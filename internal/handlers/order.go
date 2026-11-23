package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rishabharidas/restolink/internal/middleware"
	"github.com/rishabharidas/restolink/internal/models"
	"github.com/rishabharidas/restolink/internal/realtime"
)

type OrderHandler struct {
	pool *pgxpool.Pool
	hub  *realtime.Hub
}

func NewOrderHandler(pool *pgxpool.Pool, hub *realtime.Hub) *OrderHandler {
	return &OrderHandler{pool: pool, hub: hub}
}

type CreateOrderItemRequest struct {
	ItemID              uuid.UUID `json:"item_id"`
	Quantity            int       `json:"quantity"`
	SpecialInstructions string    `json:"special_instructions,omitempty"`
}

type CreateOrderRequest struct {
	OrderType           models.OrderType         `json:"order_type"` // DINE_IN, DELIVERY, TAKEAWAY
	TableID             *uuid.UUID               `json:"table_id,omitempty"`
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
		http.Error(w, `{"error":"not_found","message":"tenant not found"}`, http.StatusNotFound)
		return
	}

	var req CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad_request","message":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if len(req.Items) == 0 {
		http.Error(w, `{"error":"bad_request","message":"order must contain at least one item"}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		http.Error(w, `{"error":"server_error","message":"failed to begin transaction"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(ctx)

	// Fetch tenant default outlet
	var outletID uuid.UUID
	err = tx.QueryRow(ctx, "SELECT id FROM restoapp.outlets WHERE tenant_id = $1 LIMIT 1", tenantID).Scan(&outletID)
	if err != nil {
		http.Error(w, `{"error":"server_error","message":"outlet not found for tenant"}`, http.StatusInternalServerError)
		return
	}

	// Calculate prices and validate items
	var subtotal float64
	type itemCalculated struct {
		ItemID    uuid.UUID
		Name      string
		UnitPrice float64
		Quantity  int
		ItemTotal float64
		Notes     string
	}
	var calculatedItems []itemCalculated

	for _, reqItem := range req.Items {
		var price float64
		var name string
		err := tx.QueryRow(ctx, "SELECT name, price FROM restoapp.menu_items WHERE id = $1 AND tenant_id = $2", reqItem.ItemID, tenantID).Scan(&name, &price)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"bad_request","message":"invalid item id %s"}`, reqItem.ItemID), http.StatusBadRequest)
			return
		}
		itemTotal := price * float64(reqItem.Quantity)
		subtotal += itemTotal
		calculatedItems = append(calculatedItems, itemCalculated{
			ItemID:    reqItem.ItemID,
			Name:      name,
			UnitPrice: price,
			Quantity:  reqItem.Quantity,
			ItemTotal: itemTotal,
			Notes:     reqItem.SpecialInstructions,
		})
	}

	taxAmount := subtotal * 0.05 // 5% GST
	var deliveryFee float64
	if req.OrderType == models.OrderTypeDelivery {
		deliveryFee = 40.0 // Standard delivery fee
	}
	totalAmount := subtotal + taxAmount + deliveryFee

	orderNumber := fmt.Sprintf("ORD-%d", time.Now().Unix()%100000)
	orderID := uuid.New()

	// Insert order
	insertOrderQuery := `
		INSERT INTO restoapp.orders (
			id, tenant_id, outlet_id, order_number, order_type, status,
			table_id, special_instructions, subtotal, tax_amount, delivery_fee, total_amount, payment_status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 'PENDING')
	`
	_, err = tx.Exec(ctx, insertOrderQuery,
		orderID, tenantID, outletID, orderNumber, req.OrderType, models.StatusPlaced,
		req.TableID, req.SpecialInstructions, subtotal, taxAmount, deliveryFee, totalAmount,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"database_error","message":"failed to insert order: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Insert order items
	for _, ci := range calculatedItems {
		_, err := tx.Exec(ctx, `
			INSERT INTO restoapp.order_items (id, order_id, item_id, quantity, unit_price, item_total, item_status, special_instructions)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, uuid.New(), orderID, ci.ItemID, ci.Quantity, ci.UnitPrice, ci.ItemTotal, models.StatusPlaced, ci.Notes)
		if err != nil {
			http.Error(w, `{"error":"database_error","message":"failed to insert order items"}`, http.StatusInternalServerError)
			return
		}
	}

	// Insert initial status history
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
		http.Error(w, `{"error":"server_error","message":"failed to commit transaction"}`, http.StatusInternalServerError)
		return
	}

	// Broadcast NEW_ORDER event to tenant room (Kitchen KDS + Waiters)
	h.hub.BroadcastToRoom("tenant:"+tenantID.String(), "NEW_ORDER", map[string]interface{}{
		"order_id":     orderID,
		"order_number": orderNumber,
		"order_type":   req.OrderType,
		"total_amount": totalAmount,
		"status":       models.StatusPlaced,
	})

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
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

type UpdateStatusRequest struct {
	Status models.OrderStatus `json:"status"`
	Note   string             `json:"note,omitempty"`
}

func (h *OrderHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	orderIDStr := chi.URLParam(r, "id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		http.Error(w, `{"error":"bad_request","message":"invalid order id"}`, http.StatusBadRequest)
		return
	}

	var req UpdateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad_request","message":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Get current status & tenant ID
	var currentStatus models.OrderStatus
	var tenantID uuid.UUID
	err = h.pool.QueryRow(ctx, "SELECT status, tenant_id FROM restoapp.orders WHERE id = $1", orderID).Scan(&currentStatus, &tenantID)
	if err != nil {
		http.Error(w, `{"error":"not_found","message":"order not found"}`, http.StatusNotFound)
		return
	}

	// Update order status
	_, err = h.pool.Exec(ctx, "UPDATE restoapp.orders SET status = $1, updated_at = NOW() WHERE id = $2", req.Status, orderID)
	if err != nil {
		http.Error(w, `{"error":"database_error","message":"failed to update order status"}`, http.StatusInternalServerError)
		return
	}

	// Append to status history
	_, _ = h.pool.Exec(ctx, `
		INSERT INTO restoapp.order_status_history (order_id, previous_status, new_status, note)
		VALUES ($1, $2, $3, $4)
	`, orderID, currentStatus, req.Status, req.Note)

	// Broadcast ORDER_STATUS_UPDATED to:
	// 1. Specific Order room (Customer live view & tracker)
	// 2. Tenant room (KDS and Waiters)
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

func (h *OrderHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	orderIDStr := chi.URLParam(r, "id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		http.Error(w, `{"error":"bad_request","message":"invalid order id"}`, http.StatusBadRequest)
		return
	}

	var ord models.Order
	query := `
		SELECT o.id, o.tenant_id, o.outlet_id, o.order_number, o.order_type, o.status,
		       o.subtotal, o.tax_amount, o.delivery_fee, o.total_amount, o.payment_status,
		       o.created_at, o.updated_at
		FROM restoapp.orders o
		WHERE o.id = $1
	`
	err = h.pool.QueryRow(r.Context(), query, orderID).Scan(
		&ord.ID, &ord.TenantID, &ord.OutletID, &ord.OrderNumber, &ord.OrderType, &ord.Status,
		&ord.Subtotal, &ord.TaxAmount, &ord.DeliveryFee, &ord.TotalAmount, &ord.PaymentStatus,
		&ord.CreatedAt, &ord.UpdatedAt,
	)
	if err != nil {
		http.Error(w, `{"error":"not_found","message":"order not found"}`, http.StatusNotFound)
		return
	}

	// Fetch items
	rows, err := h.pool.Query(r.Context(), `
		SELECT oi.id, oi.item_id, mi.name, oi.quantity, oi.unit_price, oi.item_total, oi.item_status, COALESCE(oi.special_instructions, '')
		FROM restoapp.order_items oi
		JOIN restoapp.menu_items mi ON oi.item_id = mi.id
		WHERE oi.order_id = $1
	`, orderID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var it models.OrderItem
			if err := rows.Scan(&it.ID, &it.ItemID, &it.ItemName, &it.Quantity, &it.UnitPrice, &it.ItemTotal, &it.ItemStatus, &it.SpecialInstructions); err == nil {
				ord.Items = append(ord.Items, it)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ord)
}
