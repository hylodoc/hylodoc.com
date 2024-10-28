package billing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/checkout/session"
	"github.com/stripe/stripe-go/v72/webhook"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
)

func (b *BillingService) StripeWebhook() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("stripe webhook...")

		if err := b.stripeWebhook(w, r); err != nil {
			log.Printf("error processing event: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)

			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (b *BillingService) stripeWebhook(w http.ResponseWriter, r *http.Request) error {
	const MaxBodyBytes = int64(65536)
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("error reading body: %w", err)
	}

	log.Println("verifying stripe event signature...")
	_, err = webhook.ConstructEvent(payload, r.Header.Get("Stripe-Signature"), config.Config.Stripe.WebhookSigningSecret)
	if err != nil {
		fmt.Errorf("error verifying webhook signature: %w", err)
	}
	log.Println("stripe event signature verified.")

	event := stripe.Event{}
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("error unpacking payload into event: %w", err)
	}
	log.Printf("%s\n", eventString(&event))

	switch event.Type {
	case "checkout.session.completed":
		return handleCheckoutSessionCompleted(b.store, &event)
	case "checkout.session.expired":
		return handleCheckoutSessionExpired(b.store, &event)
	case "customer.subscription.created":
		return handleCustomerSubscriptionCreated(b.store, &event)
	case "customer.subscription.deleted":
		return handleCustomerSubscriptionDeleted(b.store, &event)
	case "customer.subscription.updated":
		return handleCustomerSubscriptionUpdated(b.store, &event)
	default:
		log.Printf("unhandled event type: %s\n", event.Type)
		log.Printf("event: %v\n", event)
	}
	return nil
}

type CheckoutSessionCompletedEvent struct {
	ID             string `json:"id"`
	SubscriptionID string `json:"subscription"`
}

func handleCheckoutSessionCompleted(s *model.Store, e *stripe.Event) error {
	log.Println("checkout.session.completed event...")

	var checkoutSession stripe.CheckoutSession
	if err := json.Unmarshal(e.Data.Raw, &checkoutSession); err != nil {
		return fmt.Errorf("error unmarshaling: %w", err)
	}
	log.Printf("checkout.session.completed event: %v\n", checkoutSession)

	checkoutSessionID := checkoutSession.ID

	/* get pending session to associate subscription with user */
	pending, err := s.GetPendingStripeCheckoutSession(context.TODO(), checkoutSessionID)
	if err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf("error getting pending session: %w", err)
		}
		log.Println("no pending session with user")
		return nil
	}

	/* fetch subscription since Expandable and not included in event */
	params := &stripe.CheckoutSessionParams{}
	params.AddExpand("subscription")
	params.AddExpand("line_items")
	expandedSession, err := session.Get(checkoutSessionID, params)
	if err != nil {
		log.Fatalf("Failed to retrieve checkout session: %v", err)
	}
	if expandedSession.Subscription == nil {
		return fmt.Errorf("no subscription: %w", err)
	}
	sub := expandedSession.Subscription

	/* create subscription */
	_, err = s.CreateStripeSubscription(context.TODO(), model.CreateStripeSubscriptionParams{
		UserID:               pending.UserID,
		StripeSubscriptionID: sub.ID,
		StripeCustomerID:     sub.Customer.ID,
		StripePriceID:        sub.Plan.ID,
		Amount:               sub.Plan.Amount,
		Status:               string(sub.Status),
		CurrentPeriodStart:   time.Unix(sub.CurrentPeriodStart, 0),
		CurrentPeriodEnd:     time.Unix(sub.CurrentPeriodEnd, 0),
	})
	if err != nil {
		return fmt.Errorf("error writing stripe subscription to db: %w", err)
	}

	/* update pending session */
	_, err = s.UpdateStripeCheckoutSession(context.TODO(), model.UpdateStripeCheckoutSessionParams{
		Status:          "completed",
		StripeSessionID: checkoutSessionID,
	})
	if err != nil {
		return fmt.Errorf("error updating checkout session: %w", err)
	}
	return nil
}

func handleCheckoutSessionExpired(s *model.Store, e *stripe.Event) error {
	log.Println("checkout.session.expired event...")

	/* XXX: handle*/

	return nil
}

func handleCustomerSubscriptionCreated(s *model.Store, e *stripe.Event) error {
	log.Println("customer.subscription.created event...")

	var sub stripe.Subscription
	if err := json.Unmarshal(e.Data.Raw, &sub); err != nil {
		return fmt.Errorf("error unmarshalling: %w", err)
	}
	log.Printf("customer.subscription.created event: %v", sub)

	/* XXX: we handle this on the checkout session completed event */

	return nil
}

func handleCustomerSubscriptionDeleted(s *model.Store, e *stripe.Event) error {
	log.Println("customer.subscription.deleted event...")

	/* handle subscription deleted */

	return nil
}

func handleCustomerSubscriptionUpdated(s *model.Store, e *stripe.Event) error {
	log.Println("customer.subscription.updated event...")

	var sub stripe.Subscription
	if err := json.Unmarshal(e.Data.Raw, &sub); err != nil {
		return fmt.Errorf("error unmarshalling: %w", err)
	}
	log.Printf("customer.subscription.updated event: %v", sub)

	_, err := s.UpdateStripeSubscription(context.TODO(), model.UpdateStripeSubscriptionParams{
		StripeSubscriptionID: sub.ID,
		StripePriceID:        sub.Plan.ID,
		Status:               string(sub.Status),
		Amount:               sub.Plan.Amount,
		CurrentPeriodStart:   time.Unix(sub.CurrentPeriodStart, 0),
		CurrentPeriodEnd:     time.Unix(sub.CurrentPeriodEnd, 0),
	})
	if err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf("error writing stripe subscription update: %w", err)
		}
		/* can have no rows on initial subscribe */
	}
	return nil
}

func eventString(e *stripe.Event) string {
	return fmt.Sprintf(
		"EventID: %s\nEventType: %s\nCreatedAt: %d\nData Object:%vs\n",
		e.ID, e.Type, e.Created, e.Data.Object,
	)
}
