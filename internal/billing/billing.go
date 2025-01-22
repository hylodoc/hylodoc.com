package billing

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hylodoc/hylodoc.com/internal/app/handler/request"
	"github.com/hylodoc/hylodoc.com/internal/app/handler/response"
	"github.com/hylodoc/hylodoc.com/internal/authz"
	"github.com/hylodoc/hylodoc.com/internal/config"
	"github.com/hylodoc/hylodoc.com/internal/model"
	"github.com/hylodoc/hylodoc.com/internal/session"
	"github.com/hylodoc/hylodoc.com/internal/util"

	"github.com/stripe/stripe-go/v81"
	bSession "github.com/stripe/stripe-go/v81/billingportal/session"
	"github.com/stripe/stripe-go/v81/customer"
	"github.com/stripe/stripe-go/v81/subscription"
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
	sesh := r.Session()
	sesh.Println("Pricing handler...")

	r.MixpanelTrack("Pricing")

	return response.NewTemplate(
		[]string{"pricing.html"},
		util.PageInfo{
			Data: struct {
				Title    string
				UserInfo *session.UserInfo
				Features []authz.Feature
				Tiers    []authz.Tier
			}{
				Title:    "Pricing",
				UserInfo: session.ConvertSessionToUserInfo(r.Session()),
				Features: authz.GetFeatures(),
				Tiers:    authz.GetTiers(),
			},
		},
	), nil
}

func AutoSubscribeToFreePlan(
	user *model.User, s *model.Store, sesh *session.Session,
) error {
	sesh.Println("AutoSubscribeToFreePlan...")

	/* create customer in stripe */
	cust, err := createCustomer(user.Email)
	if err != nil {
		return fmt.Errorf("error creating stripe customer: %w", err)
	}
	sesh.Printf("StripeCustomer: %v\n", cust)

	/* create subscription for customer */
	sub, err := createSubscription(
		cust.ID, config.Config.Stripe.FreePlanPriceID,
	)
	if err != nil {
		return fmt.Errorf("error creating subscription: %w", err)
	}
	sesh.Printf("StripeSubscription: %v\n", sub)

	/* write to db */
	dbsub, err := s.CreateStripeSubscription(
		context.TODO(),
		model.CreateStripeSubscriptionParams{
			UserID:               user.ID,
			SubName:              model.SubNameBasic,
			StripeCustomerID:     cust.ID,
			StripeSubscriptionID: sub.ID,
			StripeStatus:         string(sub.Status),
		},
	)
	sesh.Printf("Db sub: %v\n", dbsub)
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
	sesh := r.Session()
	sesh.Println("BillingPortal handler...")

	r.MixpanelTrack("BillingPortal")

	url, err := b.billingPortal(r)
	if err != nil {
		return nil, err
	}
	return response.NewRedirect(url, http.StatusSeeOther), nil
}

func (b *BillingService) billingPortal(r request.Request) (string, error) {
	userid, err := r.Session().GetUserID()
	if err != nil {
		return "", fmt.Errorf("get user id: %w", err)
	}
	sub, err := b.store.GetStripeSubscriptionByUserID(
		context.TODO(), userid,
	)
	if err != nil {
		return "", fmt.Errorf("could not get subcription for user: %w", err)
	}

	params := &stripe.BillingPortalSessionParams{
		Customer: stripe.String(sub.StripeCustomerID),
		ReturnURL: stripe.String(
			fmt.Sprintf(
				"%s://%s/user/account",
				config.Config.Hylodoc.Protocol,
				config.Config.Hylodoc.RootDomain,
			),
		),
	}

	/* set private key for stripe client */
	stripe.Key = config.Config.Stripe.SecretKey
	portalSession, err := bSession.New(params)
	if err != nil {
		return "", err
	}
	return portalSession.URL, nil
}
