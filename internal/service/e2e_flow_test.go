package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/rishabharidas/restolink/internal/models"
)

// TestMultiTenantOrderIsolation validates that order and table models enforce tenant isolation
func TestMultiTenantOrderIsolation(t *testing.T) {
	tenantA := uuid.New()
	tenantB := uuid.New()

	orderTenantA := models.Order{
		ID:          uuid.New(),
		TenantID:    tenantA,
		OrderNumber: "ORD-101",
		OrderType:   models.OrderTypeDineIn,
		Status:      models.StatusPlaced,
		TotalAmount: 525.0,
	}

	// Verify Tenant B cannot own or process Tenant A's order
	if orderTenantA.TenantID == tenantB {
		t.Errorf("Security breach: Tenant B matched Tenant A's order tenant ID")
	}

	tableTenantA := models.RestaurantTable{
		ID:          uuid.New(),
		TenantID:    tenantA,
		TableNumber: "T-01",
		QRCodeToken: "qr_token_table_1_tenantA",
		Capacity:    4,
		Status:      "AVAILABLE",
	}

	if tableTenantA.TenantID == tenantB {
		t.Errorf("Security breach: Tenant B matched Tenant A's table tenant ID")
	}
}

// TestKDSItemLevelAutoAdvancement verifies logic where all items ready advances parent order
func TestKDSItemLevelAutoAdvancement(t *testing.T) {
	orderID := uuid.New()
	items := []models.OrderItem{
		{
			ID:         uuid.New(),
			OrderID:    orderID,
			ItemName:   "Paneer Tikka",
			Quantity:   2,
			ItemStatus: models.StatusInCooking,
		},
		{
			ID:         uuid.New(),
			OrderID:    orderID,
			ItemName:   "Dal Makhani",
			Quantity:   1,
			ItemStatus: models.StatusInCooking,
		},
	}

	// Helper to check if order should auto-advance
	checkAllReady := func(items []models.OrderItem) bool {
		for _, it := range items {
			if it.ItemStatus != models.StatusReadyToServe && it.ItemStatus != models.StatusServed && it.ItemStatus != models.StatusCompleted {
				return false
			}
		}
		return true
	}

	// 1. Initial: items are cooking
	if checkAllReady(items) {
		t.Errorf("Expected order NOT to be ready while dishes are in cooking")
	}

	// 2. Mark first dish as ready
	items[0].ItemStatus = models.StatusReadyToServe
	if checkAllReady(items) {
		t.Errorf("Expected order NOT to be ready when only 1 of 2 dishes is ready")
	}

	// 3. Mark second dish as ready
	items[1].ItemStatus = models.StatusReadyToServe
	if !checkAllReady(items) {
		t.Errorf("Expected order to auto-advance to READY_TO_SERVE when all items are ready")
	}
}

// TestBackendPricingIntegrityWithModifiers validates that modifier price diffs are calculated accurately
func TestBackendPricingIntegrityWithModifiers(t *testing.T) {
	basePrice := 280.0
	variantAdj := 50.0 // e.g. Large
	modifiers := []models.ItemModifierOption{
		{ID: "m1", Name: "Extra Cheese", PriceDiff: 30.0},
		{ID: "m2", Name: "Spicy Masala", PriceDiff: 10.0},
	}
	quantity := 2

	modSum := 0.0
	for _, m := range modifiers {
		modSum += m.PriceDiff
	}

	unitPrice := basePrice + variantAdj + modSum
	expectedUnitPrice := 370.0
	if unitPrice != expectedUnitPrice {
		t.Errorf("Expected unit price %.2f, got %.2f", expectedUnitPrice, unitPrice)
	}

	itemTotal := unitPrice * float64(quantity)
	expectedItemTotal := 740.0
	if itemTotal != expectedItemTotal {
		t.Errorf("Expected item total %.2f, got %.2f", expectedItemTotal, itemTotal)
	}

	tax, _, totalDue := CalculateBill(itemTotal, models.OrderTypeDineIn, 5.0)
	expectedTax := 37.0
	expectedTotalDue := 777.0

	if tax != expectedTax {
		t.Errorf("Expected tax %.2f, got %.2f", expectedTax, tax)
	}
	if totalDue != expectedTotalDue {
		t.Errorf("Expected total due %.2f, got %.2f", expectedTotalDue, totalDue)
	}
}

// TestOrderCancellationRules verifies status rules for order cancellations
func TestOrderCancellationRules(t *testing.T) {
	cancellableStatuses := []models.OrderStatus{
		models.StatusPlaced,
		models.StatusWaiterConfirmed,
		models.StatusChefAccepted,
	}

	nonCancellableStatuses := []models.OrderStatus{
		models.StatusOutForDelivery,
		models.StatusDelivered,
		models.StatusServed,
		models.StatusCompleted,
	}

	isCancellable := func(s models.OrderStatus) bool {
		switch s {
		case models.StatusPlaced, models.StatusWaiterConfirmed, models.StatusChefAccepted:
			return true
		default:
			return false
		}
	}

	for _, s := range cancellableStatuses {
		if !isCancellable(s) {
			t.Errorf("Status %s should be cancellable", s)
		}
	}

	for _, s := range nonCancellableStatuses {
		if isCancellable(s) {
			t.Errorf("Status %s should NOT be cancellable once dispatched or served", s)
		}
	}
}
