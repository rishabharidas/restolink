package service

import (
	"testing"

	"github.com/rishabharidas/restolink/internal/models"
)

func TestOrderStateTransitions_Valid(t *testing.T) {
	validTransitions := []struct {
		current models.OrderStatus
		target  models.OrderStatus
	}{
		{models.StatusPlaced, models.StatusWaiterConfirmed},
		{models.StatusPlaced, models.StatusChefAccepted},
		{models.StatusPlaced, models.StatusCancelled},
		{models.StatusWaiterConfirmed, models.StatusChefAccepted},
		{models.StatusWaiterConfirmed, models.StatusInCooking},
		{models.StatusChefAccepted, models.StatusInCooking},
		{models.StatusInCooking, models.StatusBeingPrepared},
		{models.StatusInCooking, models.StatusPlating},
		{models.StatusInCooking, models.StatusReadyToServe},
		{models.StatusBeingPrepared, models.StatusPlating},
		{models.StatusPlating, models.StatusReadyToServe},
		{models.StatusReadyToServe, models.StatusServed},
		{models.StatusReadyToServe, models.StatusDriverAssigned},
		{models.StatusDriverAssigned, models.StatusOutForDelivery},
		{models.StatusOutForDelivery, models.StatusDelivered},
		{models.StatusServed, models.StatusCompleted},
		{models.StatusDelivered, models.StatusCompleted},
	}

	for _, tc := range validTransitions {
		err := ValidateOrderTransition(tc.current, tc.target)
		if err != nil {
			t.Errorf("Expected transition %s -> %s to be valid, but got error: %v", tc.current, tc.target, err)
		}
	}
}

func TestOrderStateTransitions_Invalid(t *testing.T) {
	invalidTransitions := []struct {
		current models.OrderStatus
		target  models.OrderStatus
	}{
		{models.StatusPlaced, models.StatusDelivered},
		{models.StatusPlaced, models.StatusOutForDelivery},
		{models.StatusPlaced, models.StatusReadyToServe},
		{models.StatusCancelled, models.StatusInCooking},
		{models.StatusCompleted, models.StatusPlaced},
		{models.StatusDelivered, models.StatusInCooking},
		{models.StatusPlaced, models.StatusPlaced}, // No-op
	}

	for _, tc := range invalidTransitions {
		err := ValidateOrderTransition(tc.current, tc.target)
		if err == nil {
			t.Errorf("Expected transition %s -> %s to be REJECTED, but it succeeded", tc.current, tc.target)
		}
	}
}

func TestCalculateBill(t *testing.T) {
	// Test Dine-In (No delivery fee, 5% GST)
	subtotal := 500.0
	tax, deliveryFee, total := CalculateBill(subtotal, models.OrderTypeDineIn, 5.0)

	expectedTax := 25.0
	expectedDelivery := 0.0
	expectedTotal := 525.0

	if tax != expectedTax {
		t.Errorf("Expected tax %.2f, got %.2f", expectedTax, tax)
	}
	if deliveryFee != expectedDelivery {
		t.Errorf("Expected delivery fee %.2f, got %.2f", expectedDelivery, deliveryFee)
	}
	if total != expectedTotal {
		t.Errorf("Expected total %.2f, got %.2f", expectedTotal, total)
	}

	// Test Delivery (40 INR delivery fee, 5% GST)
	taxD, deliveryFeeD, totalD := CalculateBill(subtotal, models.OrderTypeDelivery, 5.0)
	if deliveryFeeD != 40.0 {
		t.Errorf("Expected delivery fee 40.0, got %.2f", deliveryFeeD)
	}
	if totalD != 565.0 {
		t.Errorf("Expected total 565.0, got %.2f", totalD)
	}
	if taxD != 25.0 {
		t.Errorf("Expected tax 25.0, got %.2f", taxD)
	}
}
