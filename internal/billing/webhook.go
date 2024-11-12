package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/price"
	"github.com/stripe/stripe-go/v78/product"
	"github.com/stripe/stripe-go/v78/subscription"
	"github.com/stripe/stripe-go/v78/webhook"
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

func handleCustomerSubscriptionCreated(
	s *model.Store, e *stripe.Event, logger *log.Logger,
) error {
	logger.Println("customer.subscription.created event...")

	var sub stripe.Subscription
	if err := json.Unmarshal(e.Data.Raw, &sub); err != nil {
		return fmt.Errorf("error unmarshalling: %w", err)
	}
	logger.Printf("customer.subscription.created event: %v", sub)

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

	expSub, err := fetchExpandedSubscription(sub.ID)
	if err != nil {
		return fmt.Errorf(
			"error fetching expanded subscription: %w", err,
		)
	}
	logger.Printf("customer.subscription.updated expanded sub: %v\n", expSub)

	logger.Printf("data: %v\n", expSub.Items.Data)

	planName, err := getCustomerProductName(sub.Customer.ID)
	if err != nil {
		return fmt.Errorf("Error getting plan name: %w", err)
	}
	logger.Printf("Plan name: %s\n", planName)

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
