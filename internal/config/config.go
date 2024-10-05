package config

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

var Config Configuration

type Configuration struct {
	Progstack ProgstackParams `mapstructure:"progstack"`
	Github    GithubParams    `mapstructure:"github"`
	Db        DbParams        `mapstructure:"postgres"`
	Resend    ResendParams    `mapstructure:"resend"`
	Stripe    StripeParams    `mapstructure:"stripe"`
}

type ProgstackParams struct {
	Protocol         string `mapstructure:"protocol"`
	ServiceName      string `mapstructure:"service_name"`
	RepositoriesPath string `mapstructure:"repositories_path"`
	WebsitesPath     string `mapstructure:"websites_path"`
	FromEmail        string `mapstructure:"from_email"`
	AccountsEmail    string `mapstructure:"accounts_email"`
}

type GithubParams struct {
	AppID          int64  `mapstructure:"app_id"`
	AppName        string `mapstructure:"app_name"`
	ClientID       string `mapstructure:"client_id"`
	ClientSecret   string `mapstructure:"client_secret"`
	WebhookSecret  string `mapstructure:"webhook_secret"`
	PrivateKeyPath string `mapstructure:"private_key_path"`
	OAuthCallback  string `mapstructure:"oauth_callback"`
	LinkCallback   string `mapstructure:"link_callback"`
}

type DbParams struct {
	Host     string `mapstructure:"host"`
	Name     string `mapstructure:"name"`
	Schema   string `mapstructure:"schema"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Port     int    `mapstructure:"port"`
}

type ResendParams struct {
	ApiKey string `mapstructure:"resend_api_key"`
}

type StripeParams struct {
	PublishableKey       string `mapstructure:"publishable_key"`
	SecretKey            string `mapstructure:"secret_key"`
	WebhookSigningSecret string `mapstructure:"webhook_signing_secret"`
	Plans                []Plan `mapstructure:"plans"`
}

type Plan struct {
	ProductID string  `mapstructure:"product_id"`
	Name      string  `mapstructure:"name"`
	Currency  string  `mapstructure:"currency"`
	Prices    []Price `mapstructure:"prices"`
}

type Price struct {
	PriceID  string `mapstructure:"price_id"`
	Amount   int64  `mapstructure:"amount"`
	Interval string `mapstructure:"interval"`
}

func (params DbParams) Connect() (*sql.DB, error) {
	connstr := fmt.Sprintf(
		"host=%s user=%s password=%s port=%d dbname=%s sslmode=disable",
		params.Host,
		params.User,
		params.Password,
		params.Port,
		params.Name,
	)
	db, err := sql.Open("postgres", connstr)
	if err != nil {
		return nil, err
	}
	return db, nil
}
