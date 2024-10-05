package billing

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/xr0-org/progstack/internal/auth"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/util"

	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/checkout/session"
)

type BillingService struct {
	store *model.Store
}

func NewBillingService(s *model.Store) *BillingService {
	return &BillingService{
		store: s,
	}
}

func (b *BillingService) CreateCheckoutSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("createCheckoutSession handler..")

		url, err := b.createCheckoutSession(w, r)
		if err != nil {
			log.Printf("error creating checkout session: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		log.Println("redirecting user to stripe for payment...")
		http.Redirect(w, r, url, http.StatusSeeOther)
	}
}

func (b *BillingService) createCheckoutSession(w http.ResponseWriter, r *http.Request) (string, error) {
	userSession, ok := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
	if !ok {
		return "", fmt.Errorf("error getting session")
	}

	priceID := r.FormValue("plan")
	log.Printf("got priceID: %s\n", priceID)

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
	checkoutSession, err := session.New(params)
	if err != nil {
		return "", fmt.Errorf("error creating stripe checkout session: %w", err)
	}

	/* write the stripeCheckoutSessionID to db */
	_, err = b.store.CreateStripeCheckoutSession(context.TODO(), model.CreateStripeCheckoutSessionParams{
		StripeSessionID: checkoutSession.ID,
		UserID:          userSession.UserID,
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
		log.Printf("payment success...")

		session, ok := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
		if !ok {
			http.Error(w, "User not found", http.StatusUnauthorized)
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
					Title   string
					Session *auth.Session
					Success SuccessParams
				}{
					Title:   "Payment Success",
					Session: session,
					Success: params,
				},
			},
			template.FuncMap{},
		)
	}
}

func (b *BillingService) Cancel() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("payment cancel...")

		session, ok := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
		if !ok {
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}

		util.ExecTemplate(w, []string{"subscription_cancel.html"},
			util.PageInfo{
				Data: struct {
					Title   string
					Session *auth.Session
				}{
					Title:   "Payment Cancel",
					Session: session,
				},
			},
			template.FuncMap{},
		)
	}
}
