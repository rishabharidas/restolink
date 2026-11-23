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
	"github.com/rishabharidas/restolink/internal/middleware"
	"github.com/rishabharidas/restolink/internal/models"
	"github.com/rishabharidas/restolink/internal/realtime"
)

type TableHandler struct {
	pool *pgxpool.Pool
	rdb  *redis.Client
	hub  *realtime.Hub
}

func NewTableHandler(pool *pgxpool.Pool, rdb *redis.Client, hub *realtime.Hub) *TableHandler {
	return &TableHandler{pool: pool, rdb: rdb, hub: hub}
}

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
		// Look up tenant from table
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

	// 1. Store in Redis recent alerts list
	if h.rdb != nil {
		alertBytes, _ := json.Marshal(alert)
		h.rdb.LPush(r.Context(), "alerts:"+tenantID.String(), alertBytes)
		h.rdb.LTrim(r.Context(), "alerts:"+tenantID.String(), 0, 49) // Keep last 50
	}

	// 2. Broadcast via WebSocket to waiter & floor staff
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

	// 1. Cache in Redis with 4-hour session TTL
	if h.rdb != nil {
		data, _ := json.Marshal(session)
		h.rdb.Set(r.Context(), "table_cart:"+tableID.String(), data, 4*time.Hour)
	}

	// 2. Broadcast to all diners at this table in sub-50ms
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

	// Generate cryptographic hash
	hasher := sha256.New()
	hasher.Write([]byte(fmt.Sprintf("%s-%s-%d", tenantID, req.TableNumber, time.Now().UnixNano())))
	token := "qr_" + hex.EncodeToString(hasher.Sum(nil))[:24]

	// Fetch outlet
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
