package models

import (
	"time"

	"github.com/google/uuid"
)

type MenuCategory struct {
	ID           uuid.UUID  `json:"id"`
	TenantID     uuid.UUID  `json:"tenant_id"`
	Name         string     `json:"name"`
	Description  string     `json:"description,omitempty"`
	DisplayOrder int        `json:"display_order"`
	IsActive     bool       `json:"is_active"`
	Items        []MenuItem `json:"items,omitempty"`
}

type MenuItem struct {
	ID                  uuid.UUID `json:"id"`
	TenantID            uuid.UUID `json:"tenant_id"`
	CategoryID          uuid.UUID `json:"category_id"`
	Name                string    `json:"name"`
	Description         string    `json:"description,omitempty"`
	Price               float64   `json:"price"`
	FoodType            string    `json:"food_type"` // VEG, NON_VEG, EGG, VEGAN
	ImageURL            string    `json:"image_url,omitempty"`
	IsAvailable         bool      `json:"is_available"`
	KitchenStation      string    `json:"kitchen_station"`
	PreparationTimeMins int       `json:"preparation_time_mins"`
	CreatedAt           time.Time `json:"created_at"`
}

type RestaurantTable struct {
	ID            uuid.UUID  `json:"id"`
	TenantID      uuid.UUID  `json:"tenant_id"`
	OutletID      uuid.UUID  `json:"outlet_id"`
	TableNumber   string     `json:"table_number"`
	QRCodeToken   string     `json:"qr_code_token"`
	Capacity      int        `json:"capacity"`
	Status        string     `json:"status"` // AVAILABLE, OCCUPIED, RESERVED
	ActiveOrderID *uuid.UUID `json:"active_order_id,omitempty"`
}
