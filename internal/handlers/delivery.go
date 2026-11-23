package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rishabharidas/restolink/internal/models"
	"github.com/rishabharidas/restolink/internal/realtime"
)

type DeliveryHandler struct {
	pool *pgxpool.Pool
	rdb  *redis.Client
	hub  *realtime.Hub
}

func NewDeliveryHandler(pool *pgxpool.Pool, rdb *redis.Client, hub *realtime.Hub) *DeliveryHandler {
	return &DeliveryHandler{pool: pool, rdb: rdb, hub: hub}
}

type IngestLocationRequest struct {
	DriverID  uuid.UUID `json:"driver_id"`
	OrderID   uuid.UUID `json:"order_id"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Heading   float32   `json:"heading"`
	Speed     float32   `json:"speed"`
}

func (h *DeliveryHandler) IngestLocation(w http.ResponseWriter, r *http.Request) {
	var req IngestLocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad_request","message":"invalid location payload"}`, http.StatusBadRequest)
		return
	}

	now := time.Now()
	location := models.DriverLocation{
		DriverID:  req.DriverID,
		OrderID:   req.OrderID,
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
		Heading:   req.Heading,
		Speed:     req.Speed,
		Timestamp: now,
	}

	// 1. In-memory fast path: Save in Redis for high-speed retrieval
	if h.rdb != nil {
		geoKey := "driver_location:" + req.DriverID.String()
		locJSON, _ := json.Marshal(location)
		h.rdb.Set(r.Context(), geoKey, locJSON, 5*time.Minute)

		// Geospatial indexing
		h.rdb.GeoAdd(r.Context(), "drivers_geo", &redis.GeoLocation{
			Name:      req.DriverID.String(),
			Longitude: req.Longitude,
			Latitude:  req.Latitude,
		})
	}

	// 2. Broadcast immediately over WebSocket to Customer's Live Map Room
	h.hub.BroadcastToRoom("order:"+req.OrderID.String(), "DRIVER_LOCATION_STREAM", location)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]bool{"recorded": true})
}

func (h *DeliveryHandler) GetTracking(w http.ResponseWriter, r *http.Request) {
	orderIDStr := chi.URLParam(r, "order_id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		http.Error(w, `{"error":"bad_request","message":"invalid order id"}`, http.StatusBadRequest)
		return
	}

	var del models.Delivery
	query := `
		SELECT d.id, d.tenant_id, d.order_id, d.driver_id, d.delivery_address,
		       COALESCE(d.delivery_lat, 0), COALESCE(d.delivery_lng, 0), d.customer_phone
		FROM restoapp.deliveries d
		WHERE d.order_id = $1
	`
	err = h.pool.QueryRow(r.Context(), query, orderID).Scan(
		&del.ID, &del.TenantID, &del.OrderID, &del.DriverID, &del.DeliveryAddress,
		&del.DeliveryLat, &del.DeliveryLng, &del.CustomerPhone,
	)
	if err != nil {
		http.Error(w, `{"error":"not_found","message":"delivery details not found"}`, http.StatusNotFound)
		return
	}

	// Fetch latest driver location from Redis if driver assigned
	var latestLoc *models.DriverLocation
	if del.DriverID != nil && h.rdb != nil {
		val, err := h.rdb.Get(r.Context(), "driver_location:"+del.DriverID.String()).Result()
		if err == nil {
			var loc models.DriverLocation
			if err := json.Unmarshal([]byte(val), &loc); err == nil {
				latestLoc = &loc
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"delivery":       del,
		"latest_location": latestLoc,
		"ws_room":        fmt.Sprintf("order:%s", orderID),
	})
}
