package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type WebsiteSection struct {
	ID           uuid.UUID       `json:"id"`
	PageID       uuid.UUID       `json:"page_id"`
	TenantID     uuid.UUID       `json:"tenant_id"`
	SectionType  string          `json:"section_type"`
	DisplayOrder int             `json:"display_order"`
	IsEnabled    bool            `json:"is_enabled"`
	Content      json.RawMessage `json:"content"`
	Settings     json.RawMessage `json:"settings"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type WebsitePage struct {
	ID              uuid.UUID        `json:"id"`
	TenantID        uuid.UUID        `json:"tenant_id"`
	Slug            string           `json:"slug"`
	Title           string           `json:"title"`
	IsPublished     bool             `json:"is_published"`
	MetaDescription string           `json:"meta_description,omitempty"`
	Sections        []WebsiteSection `json:"sections,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}
