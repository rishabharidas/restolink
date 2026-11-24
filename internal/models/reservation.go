package models

import (
	"time"

	"github.com/google/uuid"
)

type ReservationStatus string

const (
	ReservationPending   ReservationStatus = "PENDING"
	ReservationConfirmed ReservationStatus = "CONFIRMED"
	ReservationSeated    ReservationStatus = "SEATED"
	ReservationCancelled ReservationStatus = "CANCELLED"
)

type Reservation struct {
	ID              uuid.UUID         `json:"id"`
	TenantID        uuid.UUID         `json:"tenant_id"`
	OutletID        uuid.UUID         `json:"outlet_id"`
	CustomerName    string            `json:"customer_name"`
	CustomerPhone   string            `json:"customer_phone"`
	CustomerEmail   *string           `json:"customer_email,omitempty"`
	GuestCount      int               `json:"guest_count"`
	ReservationDate string            `json:"reservation_date"` // YYYY-MM-DD
	ReservationTime string            `json:"reservation_time"` // HH:MM
	Status          ReservationStatus `json:"status"`
	TableID         *uuid.UUID        `json:"table_id,omitempty"`
	TableNumber     string            `json:"table_number,omitempty"`
	SpecialRequests string            `json:"special_requests,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}
