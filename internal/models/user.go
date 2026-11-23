package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID `json:"id"`
	TenantID     *uuid.UUID `json:"tenant_id,omitempty"`
	RoleID       int       `json:"role_id"`
	RoleName     string    `json:"role_name,omitempty"`
	FirstName    string    `json:"first_name"`
	LastName     string    `json:"last_name"`
	Email        *string   `json:"email,omitempty"`
	Phone        string    `json:"phone"`
	PasswordHash string    `json:"-"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
}

type Delivery struct {
	ID                   uuid.UUID  `json:"id"`
	TenantID             uuid.UUID  `json:"tenant_id"`
	OrderID              uuid.UUID  `json:"order_id"`
	DriverID             *uuid.UUID `json:"driver_id,omitempty"`
	DriverName           string     `json:"driver_name,omitempty"`
	DriverPhone          string     `json:"driver_phone,omitempty"`
	DeliveryAddress      string     `json:"delivery_address"`
	DeliveryLat          float64    `json:"delivery_lat"`
	DeliveryLng          float64    `json:"delivery_lng"`
	CustomerPhone        string     `json:"customer_phone"`
	DeliveryOTP          string     `json:"delivery_otp,omitempty"`
	EstimatedArrivalTime *time.Time `json:"estimated_arrival_time,omitempty"`
	StartedAt            *time.Time `json:"started_at,omitempty"`
	DeliveredAt          *time.Time `json:"delivered_at,omitempty"`
}

type DriverLocation struct {
	DriverID  uuid.UUID `json:"driver_id"`
	OrderID   uuid.UUID `json:"order_id"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Heading   float32   `json:"heading"`
	Speed     float32   `json:"speed"`
	Timestamp time.Time `json:"timestamp"`
}
