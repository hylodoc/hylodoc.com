package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/price"
	"github.com/stripe/stripe-go/v78/product"
	"github.com/stripe/stripe-go/v78/subscription"
	"github.com/stripe/stripe-go/v78/webhook"
	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
)

func (b *BillingService) StripeWebhook(
	r request.Request,
) (response.Response, error) {
	r.Session().Println("Stripe webhook...")
	if err := b.stripeWebhook(r); err != nil {
		return nil, err
	}
	return response.NewEmpty(), nil
}

func (b *BillingService) stripeWebhook(r request.Request) error {
	sesh := r.Session()

	payload, err := r.ReadBody()
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	if _, err := webhook.ConstructEvent(
		payload,
		r.GetHeader("Stripe-Signature"),
		config.Config.Stripe.WebhookSigningSecret,
	); err != nil {
		/* TODO: verify */
		sesh.Printf("invalid webhook signature: %s\n", err)
	}

	event, err := parseEvent(payload)
	if err != nil {
		return fmt.Errorf("parse event: %w", err)
	}
	switch event.Type {
	case "customer.subscription.created":
		return handleCustomerSubscriptionCreated(
			b.store, event, sesh,
		)
	case "customer.subscription.deleted":
		return handleCustomerSubscriptionDeleted(
			b.store, event, sesh,
		)
	case "customer.subscription.updated":
		return handleCustomerSubscriptionUpdated(
			b.store, event, sesh,
		)
	default:
		sesh.Printf(
			"unhandled event type %q: %s\n",
			event.Type, string(payload),
		)
		return nil
	}
}

func parseEvent(payload []byte) (*stripe.Event, error) {
	var event stripe.Event
	return &event, json.Unmarshal(payload, &event)
}

func handleCustomerSubscriptionCreated(
	s *model.Store, e *stripe.Event, sesh *session.Session,
) error {
	sesh.Println("customer.subscription.created event...")

	var sub stripe.Subscription
	if err := json.Unmarshal(e.Data.Raw, &sub); err != nil {
		return fmt.Errorf("error unmarshalling: %w", err)
	}
	sesh.Printf("customer.subscription.created event: %v", sub)

	return nil
}

func handleCustomerSubscriptionDeleted(
	s *model.Store, e *stripe.Event, sesh *session.Session,
) error {
	sesh.Println("customer.subscription.deleted event...")

	/* handle subscription deleted */

	return nil
}

func handleCustomerSubscriptionUpdated(
	s *model.Store, e *stripe.Event, sesh *session.Session,
) error {
	sesh.Println("customer.subscription.updated event...")

	var sub stripe.Subscription
	if err := json.Unmarshal(e.Data.Raw, &sub); err != nil {
		return fmt.Errorf("error unmarshalling: %w", err)
	}
	sesh.Printf("customer.subscription.updated event: %v", sub)

	expSub, err := fetchExpandedSubscription(sub.ID)
	if err != nil {
		return fmt.Errorf(
			"error fetching expanded subscription: %w", err,
		)
	}
	sesh.Printf("customer.subscription.updated expanded sub: %v\n", expSub)

	sesh.Printf("data: %v\n", expSub.Items.Data)

	planName, err := getCustomerProductName(sub.Customer.ID)
	if err != nil {
		return fmt.Errorf("Error getting plan name: %w", err)
	}
	sesh.Printf("Plan name: %s\n", planName)

	/* update subscription */
	if err := s.UpdateStripeSubscription(
		context.TODO(),
		model.UpdateStripeSubscriptionParams{
			StripeCustomerID:     sub.Customer.ID,
			SubName:              model.SubName(planName),
			StripeSubscriptionID: sub.ID,
			StripeStatus:         string(sub.Status),
		},
	); err != nil {
		return fmt.Errorf("error writing stripe subscription to db: %w", err)
	}

	return nil
}

func getCustomerProductName(customerID string) (string, error) {
	params := &stripe.SubscriptionListParams{
		Customer: stripe.String(customerID),
		Status:   stripe.String("active"),
	}
	params.Limit = stripe.Int64(1)
	subscriptions := subscription.List(params)

	for subscriptions.Next() {
		s := subscriptions.Subscription()

		if len(s.Items.Data) > 0 {
			subscriptionItem := s.Items.Data[0]

			priceID := subscriptionItem.Price.ID
			log.Printf("Price ID: %s", priceID)

			priceDetails, err := price.Get(priceID, nil)
			if err != nil {
				return "", fmt.Errorf("error fetching price details: %w", err)
			}

			productID := priceDetails.Product.ID
			productDetails, err := product.Get(productID, nil)
			if err != nil {
				return "", fmt.Errorf("error fetching product details: %w", err)
			}
			return productDetails.Name, nil
		}
	}
	return "", fmt.Errorf("no active subscription found for customer %s", customerID)
}

func fetchExpandedSubscription(subID string) (*stripe.Subscription, error) {
	params := &stripe.SubscriptionParams{}
	params.AddExpand("items.data")
	params.AddExpand("customer")

	expandedSub, err := subscription.Get(subID, params)
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
