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
	"github.com/stripe/stripe-go/v72/sub"
	"github.com/stripe/stripe-go/v72/webhook"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
)

func (b *BillingService) StripeWebhook() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("Stripe webhook...")

		if err := b.stripeWebhook(w, r); err != nil {
			logger.Printf("error processing event: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)

			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (b *BillingService) stripeWebhook(w http.ResponseWriter, r *http.Request) error {
	logger := logging.Logger(r)

	const MaxBodyBytes = int64(65536)
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("error reading body: %w", err)
	}

	logger.Println("verifying stripe event signature...")
	_, err = webhook.ConstructEvent(
		payload,
		r.Header.Get("Stripe-Signature"),
		config.Config.Stripe.WebhookSigningSecret,
	)
	if err != nil {
		fmt.Errorf("error verifying webhook signature: %w", err)
	}
	logger.Println("stripe event signature verified.")

	event := stripe.Event{}
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("error unpacking payload into event: %w", err)
	}
	logger.Printf("%s\n", eventString(&event))

	switch event.Type {
	case "checkout.session.completed":
		return handleCheckoutSessionCompleted(b.store, &event, logger)
	case "checkout.session.expired":
		return handleCheckoutSessionExpired(b.store, &event, logger)
	case "customer.subscription.created":
		return handleCustomerSubscriptionCreated(b.store, &event, logger)
	case "customer.subscription.deleted":
		return handleCustomerSubscriptionDeleted(b.store, &event, logger)
	case "customer.subscription.updated":
		return handleCustomerSubscriptionUpdated(b.store, &event, logger)
	default:
		logger.Printf("unhandled event type: %s\n", event.Type)
		logger.Printf("event: %v\n", event)
	}
	return nil
}

type CheckoutSessionCompletedEvent struct {
	ID             string `json:"id"`
	SubscriptionID string `json:"subscription"`
}

func handleCheckoutSessionCompleted(
	s *model.Store, e *stripe.Event, logger *log.Logger,
) error {
	logger.Println("checkout.session.completed event...")

	var checkoutSession stripe.CheckoutSession
	if err := json.Unmarshal(e.Data.Raw, &checkoutSession); err != nil {
		return fmt.Errorf("error unmarshaling: %w", err)
	}
	logger.Printf("checkout.session.completed event: %v\n", checkoutSession)

	checkoutSessionID := checkoutSession.ID

	/* get pending session to associate subscription with user */
	pending, err := s.GetPendingStripeCheckoutSession(context.TODO(), checkoutSessionID)
	if err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf("error getting pending session: %w", err)
		}
		logger.Println("no pending session with user")
		return nil
	}

	/* fetch subscription since Expandable and not included in event */
	params := &stripe.CheckoutSessionParams{}
	params.AddExpand("subscription")
	params.AddExpand("line_items")
	expandedSession, err := session.Get(checkoutSessionID, params)
	if err != nil {
		return fmt.Errorf("Failed to retrieve checkout session: %w", err)
	}
	if expandedSession.Subscription == nil {
		return fmt.Errorf("no subscription: %w", err)
	}
	sub := expandedSession.Subscription
	expSub, err := fetchExpandedSubscription(sub.ID)
	if err != nil {
		return fmt.Errorf("error fetching expanded subscription: %w", err)
	}

	logger.Printf("Sub: %v\n", expSub)
	logger.Printf("Plan: %v\n", expSub.Plan)
	logger.Printf("Product: %v\n", expSub.Plan.Product)

	/* create subscription */
	if err := s.CreateStripeSubscriptionTx(context.TODO(), model.CreateStripeSubscriptionParams{
		UserID:  pending.UserID,
		SubName: model.SubName(expSub.Plan.Product.Name),
		StripeSubscriptionID: sql.NullString{
			Valid:  true,
			String: expSub.ID,
		},
		StripeCustomerID: sql.NullString{
			Valid:  true,
			String: expSub.Customer.ID,
		},
		StripePriceID: sql.NullString{
			Valid:  true,
			String: expSub.Plan.ID,
		},
		Amount: sql.NullInt64{
			Valid: true,
			Int64: expSub.Plan.Amount,
		},
		Status: sql.NullString{
			Valid:  true,
			String: string(expSub.Status),
		},
		CurrentPeriodStart: time.Unix(expSub.CurrentPeriodStart, 0),
		CurrentPeriodEnd:   time.Unix(expSub.CurrentPeriodEnd, 0),
	}); err != nil {
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

func handleCheckoutSessionExpired(
	s *model.Store, e *stripe.Event, logger *log.Logger,
) error {
	logger.Println("checkout.session.expired event...")

	/* XXX: handle*/

	return nil
}

func handleCustomerSubscriptionCreated(
	s *model.Store, e *stripe.Event, logger *log.Logger,
) error {
	logger.Println("customer.subscription.created event...")

	var sub stripe.Subscription
	if err := json.Unmarshal(e.Data.Raw, &sub); err != nil {
		return fmt.Errorf("error unmarshalling: %w", err)
	}
	logger.Printf("customer.subscription.created event: %v", sub)

	/* XXX: we handle this on the checkout session completed event */

	return nil
}

func handleCustomerSubscriptionDeleted(
	s *model.Store, e *stripe.Event, logger *log.Logger,
) error {
	logger.Println("customer.subscription.deleted event...")

	/* handle subscription deleted */

	return nil
}

func handleCustomerSubscriptionUpdated(
	s *model.Store, e *stripe.Event, logger *log.Logger,
) error {
	logger.Println("customer.subscription.updated event...")

	var sub stripe.Subscription
	if err := json.Unmarshal(e.Data.Raw, &sub); err != nil {
		return fmt.Errorf("error unmarshalling: %w", err)
	}
	logger.Printf("customer.subscription.updated event: %v", sub)

	return nil
}

func fetchExpandedSubscription(subID string) (*stripe.Subscription, error) {
	params := &stripe.SubscriptionParams{}
	params.AddExpand("plan.product")
	params.AddExpand("customer")

	expandedSub, err := sub.Get(subID, params)
	if err != nil {
		return nil, fmt.Errorf("error fetching expanded subscription: %w", err)
	}
	return expandedSub, nil
}

func eventString(e *stripe.Event) string {
	return fmt.Sprintf(
		"EventID: %s\nEventType: %s\nCreatedAt: %d\nData Object:%vs\n",
		e.ID, e.Type, e.Created, e.Data.Object,
	)
}
