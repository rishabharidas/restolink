package models

import (
	"time"

	"github.com/google/uuid"
)

type OrderType string

const (
	OrderTypeDineIn   OrderType = "DINE_IN"
	OrderTypeDelivery OrderType = "DELIVERY"
	OrderTypeTakeaway OrderType = "TAKEAWAY"
)

type OrderStatus string

const (
	StatusPlaced          OrderStatus = "PLACED"
	StatusWaiterConfirmed OrderStatus = "WAITER_CONFIRMED"
	StatusChefAccepted    OrderStatus = "CHEF_ACCEPTED"
	StatusInCooking       OrderStatus = "IN_COOKING"
	StatusBeingPrepared   OrderStatus = "BEING_PREPARED"
	StatusPlating         OrderStatus = "PLATING"
	StatusReadyToServe    OrderStatus = "READY_TO_SERVE"
	StatusServed          OrderStatus = "SERVED"
	StatusDriverAssigned  OrderStatus = "DRIVER_ASSIGNED"
	StatusOutForDelivery  OrderStatus = "OUT_FOR_DELIVERY"
	StatusDelivered       OrderStatus = "DELIVERED"
	StatusCompleted       OrderStatus = "COMPLETED"
	StatusCancelled       OrderStatus = "CANCELLED"
)

type OrderItem struct {
	ID                  uuid.UUID            `json:"id"`
	OrderID             uuid.UUID            `json:"order_id"`
	ItemID              uuid.UUID            `json:"item_id"`
	ItemName            string               `json:"item_name,omitempty"`
	Quantity            int                  `json:"quantity"`
	UnitPrice           float64              `json:"unit_price"`
	ItemTotal           float64              `json:"item_total"`
	ItemStatus          OrderStatus          `json:"item_status"`
	AssignedStation     string               `json:"assigned_station"`
	SpecialInstructions string               `json:"special_instructions,omitempty"`
	KitchenNotes        string               `json:"kitchen_notes,omitempty"`
	CompletedAt         *time.Time           `json:"completed_at,omitempty"`
	SelectedModifiers   []ItemModifierOption `json:"selected_modifiers,omitempty"`
	CreatedAt           time.Time            `json:"created_at"`
}

type Order struct {
	ID                  uuid.UUID    `json:"id"`
	TenantID            uuid.UUID    `json:"tenant_id"`
	OutletID            uuid.UUID    `json:"outlet_id"`
	OrderNumber         string       `json:"order_number"`
	OrderType           OrderType    `json:"order_type"`
	Status              OrderStatus  `json:"status"`
	TableID             *uuid.UUID   `json:"table_id,omitempty"`
	TableNumber         string       `json:"table_number,omitempty"`
	CustomerID          *uuid.UUID   `json:"customer_id,omitempty"`
	CustomerName        string       `json:"customer_name,omitempty"`
	CustomerPhone       string       `json:"customer_phone,omitempty"`
	WaiterID            *uuid.UUID   `json:"waiter_id,omitempty"`
	IdempotencyKey      *string      `json:"idempotency_key,omitempty"`
	SpecialInstructions string       `json:"special_instructions,omitempty"`
	Subtotal            float64      `json:"subtotal"`
	TaxAmount           float64      `json:"tax_amount"`
	DeliveryFee         float64      `json:"delivery_fee"`
	TotalAmount         float64      `json:"total_amount"`
	PaymentStatus       string       `json:"payment_status"`
	PaymentMethod       *string      `json:"payment_method,omitempty"`
	Items               []OrderItem  `json:"items,omitempty"`
	CreatedAt           time.Time    `json:"created_at"`
	UpdatedAt           time.Time    `json:"updated_at"`
}

type OrderStatusHistory struct {
	ID             int64        `json:"id"`
	OrderID        uuid.UUID    `json:"order_id"`
	PreviousStatus *OrderStatus `json:"previous_status,omitempty"`
	NewStatus      OrderStatus  `json:"new_status"`
	ChangedByUserID *uuid.UUID  `json:"changed_by_user_id,omitempty"`
	Note           string       `json:"note,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
}
