package commerce

import "time"

// Order records a Checkout Session purchase intent / completion.
type Order struct {
	ID                    uint      `gorm:"primaryKey"`
	CreatedAt             time.Time `gorm:"not null"`
	UpdatedAt             time.Time `gorm:"not null"`
	SKU                   string    `gorm:"size:128;not null;index"`
	LicenseTemplate       string    `gorm:"size:128;not null"`
	StripeMode            string    `gorm:"size:32;not null"`
	CheckoutSessionID     string    `gorm:"size:128;uniqueIndex"`
	StripeCustomerID      string    `gorm:"size:128"`
	StripePaymentIntentID string    `gorm:"size:128"`
	StripeSubscriptionID  string    `gorm:"size:128"`
	StripePriceID         string    `gorm:"size:128;not null"`
	AmountCents           int64     `gorm:"not null"`
	Currency              string    `gorm:"size:16;not null"`
	Status                string    `gorm:"size:32;not null;index"`
	CustomerEmail         string    `gorm:"size:320"`
	CustomerAlias         string    `gorm:"size:128"`
}

// StripeEvent stores processed Stripe event IDs for idempotency.
type StripeEvent struct {
	ID          uint      `gorm:"primaryKey"`
	CreatedAt   time.Time `gorm:"not null"`
	EventID     string    `gorm:"size:128;uniqueIndex;not null"`
	EventType   string    `gorm:"size:128;not null"`
	ProcessedAt time.Time `gorm:"not null"`
	Outcome     string    `gorm:"size:64;not null"`
	SKU         string    `gorm:"size:128"`
	OrderID     uint      `gorm:"index"`
}

// Entitlement is the commerce-side entitlement row (safe scalars only).
type Entitlement struct {
	ID                uint      `gorm:"primaryKey"`
	CreatedAt         time.Time `gorm:"not null"`
	UpdatedAt         time.Time `gorm:"not null"`
	SKU               string    `gorm:"size:128;not null;index"`
	LicenseTemplate   string    `gorm:"size:128;not null"`
	Status            string    `gorm:"size:32;not null;index"`
	OrderID           uint      `gorm:"index"`
	StripeEventID     string    `gorm:"size:128;uniqueIndex"`
	CheckoutSessionID string    `gorm:"size:128;index"`
	StripeCustomerID  string    `gorm:"size:128"`
	CustomerAlias     string    `gorm:"size:128;index"`
	CustomerEmail     string    `gorm:"size:320"`
	FleetJobID        string    `gorm:"size:128"`
	FleetCustomerID   string    `gorm:"size:128;index"`
	RevokedAt         *time.Time
	RevokeReason      string `gorm:"size:64"`
}

// FulfillmentJob tracks Fleet bootstrap work for auto-provision SKUs.
type FulfillmentJob struct {
	ID              uint      `gorm:"primaryKey"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
	EntitlementID   uint      `gorm:"uniqueIndex;not null"`
	SKU             string    `gorm:"size:128;not null"`
	Status          string    `gorm:"size:32;not null;index"`
	FleetCustomerID string    `gorm:"size:128"`
	FleetJobID      string    `gorm:"size:128"`
	FleetHostname   string    `gorm:"size:256"`
	AttemptCount    int       `gorm:"not null"`
	LastErrorClass  string    `gorm:"size:64"`
	StartedAt       *time.Time
	CompletedAt     *time.Time
}

// RelationalModels returns GORM models for AutoMigrate (scalar columns only).
func RelationalModels() []any {
	return []any{
		&Order{},
		&StripeEvent{},
		&Entitlement{},
		&FulfillmentJob{},
	}
}

const (
	OrderStatusCreated  = "created"
	OrderStatusPaid     = "paid"
	OrderStatusFailed   = "failed"
	OrderStatusCanceled = "canceled"

	EntitlementStatusActive          = "active"
	EntitlementStatusRevokedPending  = "revoked_pending"
	EntitlementStatusProvisionQueued = "provision_queued"
	EntitlementStatusProvisioned     = "provisioned"
	EntitlementStatusProvisionFailed = "provision_failed"

	FulfillmentStatusQueued    = "queued"
	FulfillmentStatusRunning   = "running"
	FulfillmentStatusSucceeded = "succeeded"
	FulfillmentStatusFailed    = "failed"
	FulfillmentStatusSkipped   = "skipped"

	StripeEventOutcomeProcessed = "processed"
	StripeEventOutcomeDuplicate = "duplicate"
	StripeEventOutcomeRejected  = "rejected"
)
