package models

import (
	"time"

	"github.com/google/uuid"
)

type ItemModifierOption struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	PriceDiff float64 `json:"price_diff"`
}

type ItemVariant struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	PriceAdjustment float64 `json:"price_adjustment"`
}

type ItemModifierGroup struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"` // e.g. "Size", "Spice Level", "Add-ons"
	IsRequired bool                 `json:"is_required"`
	MinSelect  int                  `json:"min_select"`
	MaxSelect  int                  `json:"max_select"`
	Options    []ItemModifierOption `json:"options"`
}

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
	ID                  uuid.UUID           `json:"id"`
	TenantID            uuid.UUID           `json:"tenant_id"`
	CategoryID          uuid.UUID           `json:"category_id"`
	Name                string              `json:"name"`
	Description         string              `json:"description,omitempty"`
	Price               float64             `json:"price"`
	FoodType            string              `json:"food_type"` // VEG, NON_VEG, EGG, VEGAN
	ImageURL            string              `json:"image_url,omitempty"`
	IsAvailable         bool                `json:"is_available"`
	KitchenStation      string              `json:"kitchen_station"`
	AssignedStations    []string            `json:"assigned_stations"`
	Allergens           []string            `json:"allergens"`
	PreparationTimeMins int                 `json:"preparation_time_mins"`
	Variants            []ItemVariant       `json:"variants,omitempty"`
	Modifiers           []ItemModifierGroup `json:"modifiers,omitempty"`
	CreatedAt           time.Time           `json:"created_at"`
}

type OutletMenuItem struct {
	ID          uuid.UUID `json:"id"`
	TenantID    uuid.UUID `json:"tenant_id"`
	OutletID    uuid.UUID `json:"outlet_id"`
	ItemID      uuid.UUID `json:"item_id"`
	Price       float64   `json:"price"`
	IsAvailable bool      `json:"is_available"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
