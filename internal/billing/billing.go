package billing

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/authz"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"

	"github.com/stripe/stripe-go/v78"
	bSession "github.com/stripe/stripe-go/v78/billingportal/session"
	"github.com/stripe/stripe-go/v78/customer"
	"github.com/stripe/stripe-go/v78/subscription"
)

type BillingService struct {
	store *model.Store
}

func NewBillingService(
	s *model.Store,
) *BillingService {
	/* set private key for stripe client */
	stripe.Key = config.Config.Stripe.SecretKey

	return &BillingService{
		store: s,
	}
}

func (b *BillingService) Pricing(r request.Request) (response.Response, error) {
	logger := r.Logger()
	logger.Println("Pricing handler...")

	r.MixpanelTrack("Pricing")

	return response.NewTemplate(
		[]string{"pricing.html"},
		util.PageInfo{
			Data: struct {
				Title             string
				UserInfo          *session.UserInfo
				TierNames         []string
				Features          []string
				SubscriptionTiers []authz.SubscriptionFeatures
			}{
				Title:    "Pricing",
				UserInfo: session.ConvertSessionToUserInfo(r.Session()),
				TierNames: []string{
					"Scout",
					"Wayfarer",
					"Voyager",
					"Pathfinder",
				},
				Features: []string{
					"projects",
					"storage",
					"visitorsPerMonth",
					"customDomain",
					"themes",
					"codeStyle",
					"images",
					"emailSubscribers",
					"analytics",
					"rss",
					"likes",
					"comments",
					"teamMembers",
					"passwordProtectedPages",
					"downloadablePdfPages",
					"paidSubscribers",
				},
				SubscriptionTiers: authz.OrderedSubscriptionTiers(),
			},
		},
		template.FuncMap{
			"join": strings.Join,
		},
		logger,
	), nil
}

func AutoSubscribeToFreePlan(
	user model.User, s *model.Store, logger *log.Logger,
) error {
	logger.Println("AutoSubscribeToFreePlan...")

	/* create customer in stripe */
	cust, err := createCustomer(user.Email)
	if err != nil {
		return fmt.Errorf("error creating stripe customer: %w", err)
	}
	logger.Printf("StripeCustomer: %v\n", cust)

	/* create subscription for customer */
	sub, err := createSubscription(
		cust.ID, config.Config.Stripe.FreePlanPriceID,
	)
	if err != nil {
		return fmt.Errorf("error creating subscription: %w", err)
	}
	logger.Printf("StripeSubscription: %v\n", sub)

	/* write to db */
	dbsub, err := s.CreateStripeSubscription(
		context.TODO(),
		model.CreateStripeSubscriptionParams{
			UserID:               user.ID,
			SubName:              model.SubNameScout,
			StripeCustomerID:     cust.ID,
			StripeSubscriptionID: sub.ID,
			StripeStatus:         string(sub.Status),
		},
	)
	logger.Printf("Db sub: %v\n", dbsub)
	return nil
}

func createCustomer(email string) (*stripe.Customer, error) {
	params := &stripe.CustomerParams{
		Email: stripe.String(email),
	}
	cust, err := customer.New(params)
	if err != nil {
		return nil, err
	}
	return cust, nil
}

func createSubscription(customerID, priceID string) (*stripe.Subscription, error) {
	params := &stripe.SubscriptionParams{
		Customer: stripe.String(customerID),
		Items: []*stripe.SubscriptionItemsParams{
			{
				Price: stripe.String(priceID),
			},
		},
		PaymentBehavior: stripe.String("default_incomplete"),
	}

	subscription, err := subscription.New(params)
	if err != nil {
		return nil, err
	}
	return subscription, nil
}

func (b *BillingService) BillingPortal(
	r request.Request,
) (response.Response, error) {
	logger := r.Logger()
	logger.Println("BillingPortal handler...")

	r.MixpanelTrack("BillingPortal")

	url, err := b.billingPortal(r)
	if err != nil {
		return nil, err
	}
	return response.NewRedirect(url, http.StatusSeeOther), nil
}

func (b *BillingService) billingPortal(r request.Request) (string, error) {
	sub, err := b.store.GetStripeSubscriptionByUserID(
		context.TODO(), r.Session().GetUserID(),
	)
	if err != nil {
		return "", fmt.Errorf("could not get subcription for user: %w", err)
	}

	params := &stripe.BillingPortalSessionParams{
		Customer: stripe.String(sub.StripeCustomerID),
		ReturnURL: stripe.String(fmt.Sprintf(
			"%s://%s/user/account",
			config.Config.Progstack.Protocol,
			config.Config.Progstack.ServiceName,
		)),
	}

	/* set private key for stripe client */
	stripe.Key = config.Config.Stripe.SecretKey
	portalSession, err := bSession.New(params)
	if err != nil {
		return "", err
	}
	return portalSession.URL, nil
}
