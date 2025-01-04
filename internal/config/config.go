package config

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
	"github.com/spf13/viper"
)

func init() {
	if err := loadConfig("conf.yml"); err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
}

var Config Configuration

type Configuration struct {
	Progstack          ProgstackParams    `mapstructure:"progstack"`
	ProgstackSsg       ProgstackSsgParams `mapstructure:"progstack_ssg"`
	Github             GithubParams       `mapstructure:"github"`
	Db                 DbParams           `mapstructure:"postgres"`
	Email              EmailParams        `mapstructure:"email"`
	Stripe             StripeParams       `mapstructure:"stripe"`
	Mixpanel           MixpanelParams     `mapstructure:"mixpanel"`
	ReservedSubdomains []string           `mapstructure:"reserved_subdomains"`
}

type ProgstackParams struct {
	Progstack            string `mapstructure:"progstack"`
	Protocol             string `mapstructure:"protocol"`
	RootDomain           string `mapstructure:"root_domain"`
	RepositoriesPath     string `mapstructure:"repositories_path"`
	FoldersPath          string `mapstructure:"folders_path"`
	CertsPath            string `mapstructure:"certs_path"`
	WebsitesPath         string `mapstructure:"websites_path"`
	EmailDomain          string `mapstructure:"email_domain"`
	AccountsEmail        string `mapstructure:"accounts_email"`
	CustomDomainCNAME    string `mapstructure:"custom_domain_cname"`
	CustomDomainIP       string `mapstructure:"custom_domain_ip"`
	CustomDomainGuideURL string `mapstructure:"custom_domain_guide_url"`
	CDN                  string `mapstructure:"cdn"`
	DiscordURL           string `mapstructure:"discord_url"`
	OpenIssueURL         string `mapstructure:"open_issue_url"`
}

type ProgstackSsgParams struct {
	Themes map[string]Theme `mapstructure:"themes"`
}

type Theme struct {
	Name        string `mapstructure:"name"`
	Description string `mapstructure:"description"`
	Preview     string `mapstructure:"preview"`
	Path        string `mapstructure:"path"`
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

type EmailParams struct {
	PostmarkApiKey string `mapstructure:"postmark_api_key"`
	Queue          struct {
		MaxRetries int32         `mapstructure:"max_retries"`
		Period     time.Duration `mapstructure:"period"`
	} `mapstructure:"queue"`
}

type StripeParams struct {
	PublishableKey       string `mapstructure:"publishable_key"`
	SecretKey            string `mapstructure:"secret_key"`
	WebhookSigningSecret string `mapstructure:"webhook_signing_secret"`
	FreePlanPriceID      string `mapstructure:"free_plan_price_id"`
}

type MixpanelParams struct {
	Token string `mapstructure:"token"`
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

func loadConfig(path string) error {
	viper.SetConfigFile(path)
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return err
	}
	if err := viper.Unmarshal(&Config); err != nil {
		return err
	}
	log.Printf("loaded config: %+v\n", Config)
	return nil
}
