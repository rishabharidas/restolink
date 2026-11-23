package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type TenantBranding struct {
	PrimaryColor   string `json:"primary_color"`
	SecondaryColor string `json:"secondary_color"`
	AccentColor    string `json:"accent_color"`
	FontFamily     string `json:"font_family"`
	LogoURL        string `json:"logo_url"`
	HeroBannerURL  string `json:"hero_banner_url"`
}

type TenantSettings struct {
	Currency                string  `json:"currency"`
	CurrencySymbol          string  `json:"currency_symbol"`
	TaxGSTPercentage        float64 `json:"tax_gst_percentage"`
	ServiceChargePercentage float64 `json:"service_charge_percentage"`
	EnableDineIn            bool    `json:"enable_dine_in"`
	EnableDelivery          bool    `json:"enable_delivery"`
	EnableTakeaway          bool    `json:"enable_takeaway"`
	WhatsAppNotifications   bool    `json:"whatsapp_notifications"`
}

type Tenant struct {
	ID           uuid.UUID       `json:"id"`
	Slug         string          `json:"slug"`
	Name         string          `json:"name"`
	CustomDomain *string         `json:"custom_domain,omitempty"`
	Phone        string          `json:"phone"`
	Email        string          `json:"email"`
	Address      string          `json:"address"`
	BrandingRaw  json.RawMessage `json:"-"`
	SettingsRaw  json.RawMessage `json:"-"`
	Branding     TenantBranding  `json:"branding"`
	Settings     TenantSettings  `json:"settings"`
	IsActive     bool            `json:"is_active"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}
