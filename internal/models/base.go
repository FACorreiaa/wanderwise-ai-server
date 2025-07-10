package models

import (
	"time"

	"github.com/google/uuid"
)

// Base contains common fields used across multiple models.
type Base struct {
	ID        uuid.UUID `json:"id" db:"id"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// SubscriptionPlanType represents the type of subscription plan.
type SubscriptionPlanType string

// Subscription plan types.
const (
	SubscriptionPlanFree           SubscriptionPlanType = "free"
	SubscriptionPlanPremiumMonthly SubscriptionPlanType = "premium_monthly"
	SubscriptionPlanPremiumAnnual  SubscriptionPlanType = "premium_annual"
)

// SubscriptionStatus represents the status of a subscription
type SubscriptionStatus string

// Subscription statuses
const (
	SubscriptionStatusActive   SubscriptionStatus = "active"
	SubscriptionStatusTrialing SubscriptionStatus = "trialing"
	SubscriptionStatusPastDue  SubscriptionStatus = "past_due"
	SubscriptionStatusCanceled SubscriptionStatus = "canceled"
	SubscriptionStatusExpired  SubscriptionStatus = "expired"
)

// POISource represents the source of a point of interest
type POISource string

// POI sources
const (
	POISourceLoci          POISource = "loci_ai"
	POISourceOpenStreetMap POISource = "openstreetmap"
	POISourceUserSubmitted POISource = "user_submitted"
	POISourcePartner       POISource = "partner"
)
