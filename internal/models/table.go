package models

import (
	"time"

	"github.com/google/uuid"
)

type TableBellAction string

const (
	BellCallWaiter   TableBellAction = "CALL_WAITER"
	BellRequestWater TableBellAction = "REQUEST_WATER"
	BellRequestBill  TableBellAction = "REQUEST_BILL"
)

type TableBellRequest struct {
	TableID      uuid.UUID       `json:"table_id"`
	TableNumber  string          `json:"table_number"`
	Action       TableBellAction `json:"action"`
	CustomerName string          `json:"customer_name,omitempty"`
	Notes        string          `json:"notes,omitempty"`
}

type TableBellAlert struct {
	ID           string          `json:"id"`
	TenantID     uuid.UUID       `json:"tenant_id"`
	TableID      uuid.UUID       `json:"table_id"`
	TableNumber  string          `json:"table_number"`
	Action       TableBellAction `json:"action"`
	ActionLabel  string          `json:"action_label"`
	CustomerName string          `json:"customer_name,omitempty"`
	Notes        string          `json:"notes,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

type MultiplayerCartItem struct {
	ID                  string   `json:"id"` // Unique cart item key
	ItemID              uuid.UUID `json:"item_id"`
	ItemName            string   `json:"item_name"`
	Quantity            int      `json:"quantity"`
	UnitPrice           float64  `json:"unit_price"`
	SelectedVariant     string   `json:"selected_variant,omitempty"`
	SelectedAddons      []string `json:"selected_addons,omitempty"`
	SpecialInstructions string   `json:"special_instructions,omitempty"`
	AddedByName         string   `json:"added_by_name"`
	AddedAt             time.Time `json:"added_at"`
}

type TableSession struct {
	TableID     uuid.UUID             `json:"table_id"`
	TableNumber string                `json:"table_number"`
	TenantID    uuid.UUID             `json:"tenant_id"`
	DinersCount int                   `json:"diners_count"`
	Items       []MultiplayerCartItem `json:"items"`
	UpdatedAt   time.Time             `json:"updated_at"`
}
