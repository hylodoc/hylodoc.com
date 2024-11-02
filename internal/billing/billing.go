package billing

import (
	"context"
	"fmt"
	"html/template"
	"net/http"

	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"

	"github.com/stripe/stripe-go/v72"
	bSession "github.com/stripe/stripe-go/v72/billingportal/session"
	cSession "github.com/stripe/stripe-go/v72/checkout/session"
)

type BillingService struct {
	store *model.Store
}

func NewBillingService(s *model.Store) *BillingService {
	return &BillingService{
		store: s,
	}
}

func (b *BillingService) Subscriptions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)

		logger.Println("Subscriptions handler...")

		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			logger.Println("No auth session")
			http.Error(w, "No auth session", http.StatusUnauthorized)
			return
		}

		util.ExecTemplate(w, []string{"subscriptions.html", "subscription_product.html"},
			util.PageInfo{
				Data: struct {
					Title    string
					UserInfo *session.UserInfo
					Plans    []config.Plan
				}{
					Title:    "Subscriptions",
					UserInfo: session.ConvertSessionToUserInfo(sesh),
					Plans:    config.Config.Stripe.Plans,
				},
			},
			template.FuncMap{
				"centsToDollars": ConvertCentsToDollars,
			},
			logger,
		)
	}
}

func ConvertCentsToDollars(cents int64) string {
	dollars := float64(cents) / 100.0
	return fmt.Sprintf("$%.2f", dollars)
}

func (b *BillingService) CreateCheckoutSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)

		logger.Println("CreateCheckoutSession handler...")

		url, err := b.createCheckoutSession(w, r)
		if err != nil {
			logger.Printf("Error creating checkout session: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		logger.Println("Redirecting to stripe for payment...")
		http.Redirect(w, r, url, http.StatusSeeOther)
	}
}

func (b *BillingService) createCheckoutSession(w http.ResponseWriter, r *http.Request) (string, error) {
	logger := logging.Logger(r)

	userSession, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	if !ok {
		return "", fmt.Errorf("error getting session")
	}

	priceID := r.FormValue("plan")
	logger.Printf("PriceID: %s\n", priceID)

	params := &stripe.CheckoutSessionParams{
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			&stripe.CheckoutSessionLineItemParams{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
		Mode:       stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		SuccessURL: stripe.String(buildSuccessUrl()),
		CancelURL:  stripe.String(buildCancelUrl()),
	}

	/* set private key for stripe client */
	stripe.Key = config.Config.Stripe.SecretKey
	checkoutSession, err := cSession.New(params)
	if err != nil {
		return "", fmt.Errorf("error creating stripe checkout session: %w", err)
	}

	/* write the stripeCheckoutSessionID to db */
	_, err = b.store.CreateStripeCheckoutSession(context.TODO(), model.CreateStripeCheckoutSessionParams{
		StripeSessionID: checkoutSession.ID,
		UserID:          userSession.GetUserID(),
	})
	if err != nil {
		return "", fmt.Errorf("error writing stripe checkout session to db: %w", err)
	}
	return checkoutSession.URL, nil
}

func buildSuccessUrl() string {
	return fmt.Sprintf(
		"%s://%s/user/stripe/success",
		config.Config.Progstack.Protocol,
		config.Config.Progstack.ServiceName,
	)
}

func buildCancelUrl() string {
	return fmt.Sprintf(
		"%s://%s/user/stripe/cancel",
		config.Config.Progstack.Protocol,
		config.Config.Progstack.ServiceName,
	)
}

type SuccessParams struct {
	ServiceName  string
	ContactEmail string
}

func (b *BillingService) Success() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)

		logger.Println("Payment success!")

		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			logger.Println("No auth session")
			http.Error(w, "", http.StatusNotFound)
			return
		}

		/* XXX: need to get success params from db */
		params := SuccessParams{
			ServiceName:  config.Config.Progstack.ServiceName,
			ContactEmail: config.Config.Progstack.AccountsEmail,
		}

		util.ExecTemplate(w, []string{"subscription_success.html"},
			util.PageInfo{
				Data: struct {
					Title    string
					UserInfo *session.UserInfo
					Success  SuccessParams
				}{
					Title:    "Payment Success",
					UserInfo: session.ConvertSessionToUserInfo(sesh),
					Success:  params,
				},
			},
			template.FuncMap{},
			logger,
		)
	}
}

func (b *BillingService) Cancel() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Printf("Payment Cancel handler...")

		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			logger.Println("No auth session")
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}

		util.ExecTemplate(w, []string{"subscription_cancel.html"},
			util.PageInfo{
				Data: struct {
					Title    string
					UserInfo *session.UserInfo
				}{
					Title:    "Payment Cancel",
					UserInfo: session.ConvertSessionToUserInfo(sesh),
				},
			},
			template.FuncMap{},
			logger,
		)
	}
}

func (b *BillingService) BillingPortal() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("BillingPortal handler...")

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
