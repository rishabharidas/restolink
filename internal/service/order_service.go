package service

import (
	"fmt"

	"github.com/rishabharidas/restolink/internal/errors"
	"github.com/rishabharidas/restolink/internal/models"
)

// AllowedTransitions maps each OrderStatus to the slice of valid next OrderStatuses
var AllowedTransitions = map[models.OrderStatus][]models.OrderStatus{
	models.StatusPlaced: {
		models.StatusWaiterConfirmed,
		models.StatusChefAccepted,
		models.StatusCancelled,
	},
	models.StatusWaiterConfirmed: {
		models.StatusChefAccepted,
		models.StatusInCooking,
		models.StatusCancelled,
	},
	models.StatusChefAccepted: {
		models.StatusInCooking,
		models.StatusCancelled,
	},
	models.StatusInCooking: {
		models.StatusBeingPrepared,
		models.StatusPlating,
		models.StatusReadyToServe,
	},
	models.StatusBeingPrepared: {
		models.StatusPlating,
		models.StatusReadyToServe,
	},
	models.StatusPlating: {
		models.StatusReadyToServe,
	},
	models.StatusReadyToServe: {
		models.StatusServed,
		models.StatusDriverAssigned,
		models.StatusCompleted,
	},
	models.StatusDriverAssigned: {
		models.StatusOutForDelivery,
		models.StatusCancelled,
	},
	models.StatusOutForDelivery: {
		models.StatusDelivered,
	},
	models.StatusServed: {
		models.StatusCompleted,
	},
	models.StatusDelivered: {
		models.StatusCompleted,
	},
}

// ValidateOrderTransition checks whether transitioning from current to target status is permitted.
func ValidateOrderTransition(current, target models.OrderStatus) error {
	if current == target {
		return errors.Unprocessable("NO_OP_TRANSITION", fmt.Sprintf("Order is already in status %s", current))
	}

	if current == models.StatusCompleted || current == models.StatusCancelled {
		return errors.Unprocessable("TERMINAL_STATUS", fmt.Sprintf("Cannot transition from terminal status %s", current))
	}

	allowed, ok := AllowedTransitions[current]
	if !ok {
		return errors.Unprocessable("UNKNOWN_CURRENT_STATUS", fmt.Sprintf("Status %s has no known transitions", current))
	}

	for _, next := range allowed {
		if next == target {
			return nil
		}
	}

	return errors.Unprocessable(
		"INVALID_STATUS_TRANSITION",
		fmt.Sprintf("Invalid order state transition from '%s' to '%s'", current, target),
	)
}

// CalculateBill computes subtotal, GST tax, delivery fee, and total amount.
func CalculateBill(subtotal float64, orderType models.OrderType, gstPercentage float64) (taxAmount, deliveryFee, totalAmount float64) {
	if gstPercentage <= 0 {
		gstPercentage = 5.0 // Default 5% GST
	}
	taxAmount = subtotal * (gstPercentage / 100.0)

	deliveryFee = 0.0
	if orderType == models.OrderTypeDelivery {
		deliveryFee = 40.0 // Standard delivery fee in INR
	}

	totalAmount = subtotal + taxAmount + deliveryFee
	return taxAmount, deliveryFee, totalAmount
}
