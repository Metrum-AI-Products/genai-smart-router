package commerce

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store persists commerce orders, events, entitlements, and fulfillment jobs.
type Store struct {
	DB *gorm.DB
}

// OpenStore migrates relational commerce tables.
func OpenStore(db *gorm.DB) (*Store, error) {
	if err := db.AutoMigrate(RelationalModels()...); err != nil {
		return nil, fmt.Errorf("migrate commerce schema: %w", err)
	}
	return &Store{DB: db}, nil
}

// CreateOrder inserts a checkout order row.
func (s *Store) CreateOrder(order *Order) error {
	now := time.Now().UTC()
	order.CreatedAt = now
	order.UpdatedAt = now
	return s.DB.Create(order).Error
}

// MarkOrderPaid updates order status after verified payment.
func (s *Store) MarkOrderPaid(checkoutSessionID string, updates map[string]any) error {
	updates["updated_at"] = time.Now().UTC()
	updates["status"] = OrderStatusPaid
	return s.DB.Model(&Order{}).Where("checkout_session_id = ?", checkoutSessionID).Updates(updates).Error
}

// HasStripeEvent reports whether evt_ id was already processed.
func (s *Store) HasStripeEvent(eventID string) (bool, error) {
	var count int64
	err := s.DB.Model(&StripeEvent{}).Where("event_id = ?", eventID).Count(&count).Error
	return count > 0, err
}

// RecordStripeEvent inserts an idempotency row; unique constraint rejects duplicates.
func (s *Store) RecordStripeEvent(evt *StripeEvent) error {
	now := time.Now().UTC()
	evt.CreatedAt = now
	evt.ProcessedAt = now
	return s.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(evt).Error
}

// GetEntitlementByEvent returns entitlement for a Stripe event id.
func (s *Store) GetEntitlementByEvent(eventID string) (*Entitlement, error) {
	var ent Entitlement
	err := s.DB.Where("stripe_event_id = ?", eventID).First(&ent).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ent, nil
}

// GetEntitlementStatus returns safe scalar entitlement status by id or checkout session.
func (s *Store) GetEntitlementStatus(id uint, checkoutSessionID, customerAlias string) (*EntitlementStatus, error) {
	q := s.DB.Model(&Entitlement{})
	switch {
	case id > 0:
		q = q.Where("id = ?", id)
	case checkoutSessionID != "":
		q = q.Where("checkout_session_id = ?", checkoutSessionID)
	case customerAlias != "":
		q = q.Where("customer_alias = ?", customerAlias).Order("id desc")
	default:
		return nil, fmt.Errorf("entitlement id, checkout_session_id, or customer_alias required")
	}
	var ent Entitlement
	if err := q.First(&ent).Error; err != nil {
		return nil, err
	}
	status := &EntitlementStatus{
		ID:                ent.ID,
		SKU:               ent.SKU,
		LicenseTemplate:   ent.LicenseTemplate,
		Status:            ent.Status,
		FleetCustomerID:   ent.FleetCustomerID,
		FleetJobID:        ent.FleetJobID,
		CheckoutSessionID: ent.CheckoutSessionID,
		CreatedAt:         ent.CreatedAt.UTC().Format(time.RFC3339),
	}
	var job FulfillmentJob
	if err := s.DB.Where("entitlement_id = ?", ent.ID).First(&job).Error; err == nil {
		status.FulfillmentStatus = job.Status
		status.FleetHostname = job.FleetHostname
	}
	return status, nil
}

// EntitlementStatus is the public safe-scalar status payload.
type EntitlementStatus struct {
	ID                uint   `json:"id"`
	SKU               string `json:"sku"`
	LicenseTemplate   string `json:"license_template"`
	Status            string `json:"status"`
	FulfillmentStatus string `json:"fulfillment_status,omitempty"`
	FleetCustomerID   string `json:"fleet_customer_id,omitempty"`
	FleetJobID        string `json:"fleet_job_id,omitempty"`
	FleetHostname     string `json:"fleet_hostname,omitempty"`
	CheckoutSessionID string `json:"checkout_session_id,omitempty"`
	CreatedAt         string `json:"created_at"`
}

// ProcessPaidCheckout records entitlement and optionally enqueues Fleet fulfillment.
// Idempotent on Stripe event ID.
func (s *Store) ProcessPaidCheckout(ctx context.Context, cat *Catalog, fleet FleetRunner, eventID string, session *CheckoutSessionObject, skuName string) (*Entitlement, bool, error) {
	return s.processPaid(ctx, cat, fleet, eventID, "checkout.session.completed", session, skuName)
}

func (s *Store) processPaid(ctx context.Context, cat *Catalog, fleet FleetRunner, eventID, eventType string, session *CheckoutSessionObject, skuName string) (*Entitlement, bool, error) {
	exists, err := s.HasStripeEvent(eventID)
	if err != nil {
		return nil, false, err
	}
	if exists {
		ent, err := s.GetEntitlementByEvent(eventID)
		return ent, true, err
	}

	sku, ok := cat.LookupSKU(skuName)
	if !ok {
		_ = s.RecordStripeEvent(&StripeEvent{
			EventID: eventID, EventType: eventType,
			Outcome: StripeEventOutcomeRejected, SKU: skuName,
		})
		return nil, false, fmt.Errorf("unknown sku %q", skuName)
	}

	now := time.Now().UTC()
	ent := &Entitlement{
		CreatedAt:         now,
		UpdatedAt:         now,
		SKU:               sku.SKU,
		LicenseTemplate:   sku.LicenseTemplate,
		Status:            EntitlementStatusActive,
		StripeEventID:     eventID,
		CheckoutSessionID: session.ID,
		StripeCustomerID:  session.Customer,
		CustomerAlias:     session.ClientReferenceID,
		CustomerEmail:     session.CustomerEmail,
	}
	if alias := session.Metadata["customer_alias"]; alias != "" {
		ent.CustomerAlias = alias
	}

	err = s.DB.Transaction(func(tx *gorm.DB) error {
		evt := &StripeEvent{
			CreatedAt: now, EventID: eventID, EventType: eventType,
			ProcessedAt: now, Outcome: StripeEventOutcomeProcessed, SKU: sku.SKU,
		}
		if err := tx.Create(evt).Error; err != nil {
			return err
		}
		if session.ID != "" {
			_ = tx.Model(&Order{}).Where("checkout_session_id = ?", session.ID).Updates(map[string]any{
				"status":                   OrderStatusPaid,
				"stripe_customer_id":       session.Customer,
				"stripe_payment_intent_id": session.PaymentIntent,
				"stripe_subscription_id":   session.Subscription,
				"updated_at":               now,
			}).Error
		}
		if err := tx.Create(ent).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			ent, getErr := s.GetEntitlementByEvent(eventID)
			return ent, true, getErr
		}
		return nil, false, err
	}

	if !sku.AutoProvisionInstance {
		return ent, false, nil
	}

	customerID := ent.CustomerAlias
	if customerID == "" {
		customerID = fmt.Sprintf("buyer-%d", ent.ID)
	}
	job := &FulfillmentJob{
		CreatedAt:       now,
		UpdatedAt:       now,
		EntitlementID:   ent.ID,
		SKU:             sku.SKU,
		Status:          FulfillmentStatusQueued,
		FleetCustomerID: customerID,
	}
	if err := s.DB.Create(job).Error; err != nil {
		return ent, false, err
	}
	ent.Status = EntitlementStatusProvisionQueued
	ent.FleetCustomerID = customerID
	_ = s.DB.Model(ent).Updates(map[string]any{
		"status": ent.Status, "fleet_customer_id": customerID, "updated_at": time.Now().UTC(),
	}).Error

	if fleet == nil {
		return ent, false, nil
	}
	return ent, false, s.runFulfillment(ctx, fleet, ent, job)
}

func (s *Store) runFulfillment(ctx context.Context, fleet FleetRunner, ent *Entitlement, job *FulfillmentJob) error {
	started := time.Now().UTC()
	_ = s.DB.Model(job).Updates(map[string]any{
		"status": FulfillmentStatusRunning, "started_at": started, "attempt_count": job.AttemptCount + 1, "updated_at": started,
	}).Error
	res, err := fleet.Bootstrap(ctx, FleetBootstrapRequest{
		CustomerID:        job.FleetCustomerID,
		SKU:               ent.SKU,
		LicenseTemplate:   ent.LicenseTemplate,
		EntitlementID:     ent.ID,
		CheckoutSessionID: ent.CheckoutSessionID,
	})
	finished := time.Now().UTC()
	if err != nil {
		_ = s.DB.Model(job).Updates(map[string]any{
			"status": FulfillmentStatusFailed, "last_error_class": "fleet_bootstrap_failed",
			"completed_at": finished, "updated_at": finished,
		}).Error
		_ = s.DB.Model(ent).Updates(map[string]any{
			"status": EntitlementStatusProvisionFailed, "updated_at": finished,
		}).Error
		return err
	}
	_ = s.DB.Model(job).Updates(map[string]any{
		"status": FulfillmentStatusSucceeded, "fleet_job_id": res.JobID,
		"fleet_hostname": res.Hostname, "completed_at": finished, "updated_at": finished,
	}).Error
	_ = s.DB.Model(ent).Updates(map[string]any{
		"status": EntitlementStatusProvisioned, "fleet_job_id": res.JobID,
		"fleet_customer_id": res.CustomerID, "updated_at": finished,
	}).Error
	return nil
}

// ProcessPaidInvoice handles invoice.paid for subscriptions (first paid may provision).
func (s *Store) ProcessPaidInvoice(ctx context.Context, cat *Catalog, fleet FleetRunner, eventID string, inv *InvoiceObject, skuName string) (*Entitlement, bool, error) {
	exists, err := s.HasStripeEvent(eventID)
	if err != nil {
		return nil, false, err
	}
	if exists {
		ent, err := s.GetEntitlementByEvent(eventID)
		return ent, true, err
	}
	sku, ok := cat.LookupSKU(skuName)
	if !ok {
		_ = s.RecordStripeEvent(&StripeEvent{
			EventID: eventID, EventType: "invoice.paid",
			Outcome: StripeEventOutcomeRejected, SKU: skuName,
		})
		return nil, false, fmt.Errorf("unknown sku %q", skuName)
	}

	// For first_paid_only subscriptions, skip re-provision on renewals.
	if sku.AutoProvisionOn == AutoProvisionFirstPaidOnly || sku.AutoProvisionOn == "" {
		if inv.BillingReason == "subscription_cycle" {
			now := time.Now().UTC()
			_ = s.RecordStripeEvent(&StripeEvent{
				CreatedAt: now, EventID: eventID, EventType: "invoice.paid",
				ProcessedAt: now, Outcome: StripeEventOutcomeProcessed, SKU: sku.SKU,
			})
			return nil, false, nil
		}
	}

	session := &CheckoutSessionObject{
		ID:                inv.Metadata["checkout_session_id"],
		Customer:          inv.Customer,
		Subscription:      inv.Subscription,
		ClientReferenceID: inv.Metadata["customer_alias"],
		CustomerEmail:     inv.Metadata["customer_email"],
		Metadata:          inv.Metadata,
		AmountTotal:       inv.AmountPaid,
		Currency:          inv.Currency,
		PaymentStatus:     "paid",
		Mode:              "subscription",
	}
	return s.processPaid(ctx, cat, fleet, eventID, "invoice.paid", session, skuName)
}

// MarkEntitlementRevokedPending records refund/dispute without auto-delete.
func (s *Store) MarkEntitlementRevokedPending(checkoutSessionID, reason string) error {
	now := time.Now().UTC()
	return s.DB.Model(&Entitlement{}).Where("checkout_session_id = ?", checkoutSessionID).Updates(map[string]any{
		"status": EntitlementStatusRevokedPending, "revoked_at": now, "revoke_reason": reason, "updated_at": now,
	}).Error
}
