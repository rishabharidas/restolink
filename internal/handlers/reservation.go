package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rishabharidas/restolink/internal/errors"
	"github.com/rishabharidas/restolink/internal/middleware"
	"github.com/rishabharidas/restolink/internal/models"
	"github.com/rishabharidas/restolink/internal/realtime"
)

type ReservationHandler struct {
	pool *pgxpool.Pool
	hub  *realtime.Hub
}

func NewReservationHandler(pool *pgxpool.Pool, hub *realtime.Hub) *ReservationHandler {
	return &ReservationHandler{pool: pool, hub: hub}
}

type CreateReservationRequest struct {
	CustomerName    string `json:"customer_name"`
	CustomerPhone   string `json:"customer_phone"`
	CustomerEmail   string `json:"customer_email,omitempty"`
	GuestCount      int    `json:"guest_count"`
	ReservationDate string `json:"reservation_date"` // YYYY-MM-DD
	ReservationTime string `json:"reservation_time"` // HH:MM
	SpecialRequests string `json:"special_requests,omitempty"`
}

func (h *ReservationHandler) CreateReservation(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	var req CreateReservationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteError(w, errors.BadRequest("Invalid reservation request body"))
		return
	}

	if req.CustomerName == "" || req.CustomerPhone == "" || req.GuestCount <= 0 {
		errors.WriteError(w, errors.BadRequest("Name, phone, and valid guest count are required"))
		return
	}

	ctx := r.Context()

	// Fetch tenant default outlet
	var outletID uuid.UUID
	err := h.pool.QueryRow(ctx, "SELECT id FROM restoapp.outlets WHERE tenant_id = $1 LIMIT 1", tenantID).Scan(&outletID)
	if err != nil {
		errors.WriteError(w, errors.Internal("Outlet not configured for tenant"))
		return
	}

	reservationID := uuid.New()
	insertQuery := `
		INSERT INTO restoapp.reservations (
			id, tenant_id, outlet_id, customer_name, customer_phone, customer_email,
			guest_count, reservation_date, reservation_time, status, special_requests
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'PENDING', $10)
	`
	_, err = h.pool.Exec(ctx, insertQuery,
		reservationID, tenantID, outletID, req.CustomerName, req.CustomerPhone, req.CustomerEmail,
		req.GuestCount, req.ReservationDate, req.ReservationTime, req.SpecialRequests,
	)
	if err != nil {
		errors.WriteError(w, errors.Internal("Failed to record reservation"))
		return
	}

	// Broadcast NEW_RESERVATION to Waiters and Admin
	h.hub.BroadcastToRoom("tenant:"+tenantID.String(), "NEW_RESERVATION", map[string]interface{}{
		"reservation_id":   reservationID,
		"customer_name":    req.CustomerName,
		"customer_phone":   req.CustomerPhone,
		"guest_count":      req.GuestCount,
		"reservation_date": req.ReservationDate,
		"reservation_time": req.ReservationTime,
	})

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":        true,
		"reservation_id": reservationID,
		"status":         "PENDING",
		"message":        "Table reservation requested successfully",
	})
}

func (h *ReservationHandler) ListReservations(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		errors.WriteError(w, errors.NotFound("Tenant context not found"))
		return
	}

	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	ctx := r.Context()
	query := `
		SELECT r.id, r.tenant_id, r.outlet_id, r.customer_name, r.customer_phone,
		       COALESCE(r.customer_email, ''), r.guest_count, r.reservation_date::text,
		       r.reservation_time::text, r.status, r.table_id, COALESCE(t.table_number, ''),
		       COALESCE(r.special_requests, ''), r.created_at, r.updated_at
		FROM restoapp.reservations r
		LEFT JOIN restoapp.restaurant_tables t ON r.table_id = t.id
		WHERE r.tenant_id = $1 AND r.reservation_date = $2
		ORDER BY r.reservation_time ASC
	`
	rows, err := h.pool.Query(ctx, query, tenantID, date)
	if err != nil {
		errors.WriteError(w, errors.Internal("Failed to list reservations"))
		return
	}
	defer rows.Close()

	var reservations []models.Reservation
	for rows.Next() {
		var res models.Reservation
		var email string
		if err := rows.Scan(
			&res.ID, &res.TenantID, &res.OutletID, &res.CustomerName, &res.CustomerPhone,
			&email, &res.GuestCount, &res.ReservationDate, &res.ReservationTime,
			&res.Status, &res.TableID, &res.TableNumber, &res.SpecialRequests,
			&res.CreatedAt, &res.UpdatedAt,
		); err == nil {
			if email != "" {
				res.CustomerEmail = &email
			}
			reservations = append(reservations, res)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"reservations": reservations,
	})
}
