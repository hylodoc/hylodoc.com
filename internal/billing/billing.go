package billing

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/xr0-org/progstack/internal/analytics"
	"github.com/xr0-org/progstack/internal/authz"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"

	"github.com/stripe/stripe-go/v78"
	bSession "github.com/stripe/stripe-go/v78/billingportal/session"
	"github.com/stripe/stripe-go/v78/customer"
	"github.com/stripe/stripe-go/v78/subscription"
)

type BillingService struct {
	store    *model.Store
	mixpanel *analytics.MixpanelClientWrapper
}

func NewBillingService(
	s *model.Store, m *analytics.MixpanelClientWrapper,
) *BillingService {
	/* set private key for stripe client */
	stripe.Key = config.Config.Stripe.SecretKey

	return &BillingService{
		store:    s,
		mixpanel: m,
	}
}

func (b *BillingService) Pricing() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("Pricing handler...")

		b.mixpanel.Track("Pricing", r)

		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			logger.Println("No auth session")
			http.Error(w, "No auth session", http.StatusUnauthorized)
			return
		}

		util.ExecTemplate(w, []string{"pricing.html"},
			util.PageInfo{
				Data: struct {
					Title             string
					UserInfo          *session.UserInfo
					TierNames         []string
					Features          []string
					SubscriptionTiers []authz.SubscriptionFeatures
				}{
					Title:    "Pricing",
					UserInfo: session.ConvertSessionToUserInfo(sesh),
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
		)
	}
}

func AutoSubscribeToFreePlan(
	s *model.Store, r *http.Request, user model.User,
) error {
	logger := logging.Logger(r)
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

func (b *BillingService) BillingPortal() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("BillingPortal handler...")

		b.mixpanel.Track("BillingPortal", r)

		url, err := b.billingPortal(w, r)
		if err != nil {
			logger.Printf("error redirecting to billing portal: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		logger.Println("Redirecting User to billing portal...")
		http.Redirect(w, r, url, http.StatusSeeOther)
	}
}

func (b *BillingService) billingPortal(w http.ResponseWriter, r *http.Request) (string, error) {
	userSession, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	if !ok {
		return "", fmt.Errorf("error getting session")
	}

	sub, err := b.store.GetStripeSubscriptionByUserID(context.TODO(), userSession.GetUserID())
	if err != nil {
		return "", fmt.Errorf("could not get subcription for user: %w", err)
	}

	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(sub.StripeCustomerID),
		ReturnURL: stripe.String(fmt.Sprintf("%s://%s/user/account", config.Config.Progstack.Protocol, config.Config.Progstack.ServiceName)),
	}

	/* set private key for stripe client */
	stripe.Key = config.Config.Stripe.SecretKey
	portalSession, err := bSession.New(params)
	if err != nil {
		return "", err
	}
	return portalSession.URL, nil
}
