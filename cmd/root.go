package cmd

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/spf13/cobra"
	server "github.com/xr0-org/progstack/internal/app"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/email/emailqueue"
	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/model"
)

const clientTimeout = 30 * time.Second

var rootCmd = &cobra.Command{
	Use:   "progstack.com",
	Short: "Run progstack.com",
	RunE: func(cmd *cobra.Command, args []string) error {
		rand.Seed(time.Now().UnixNano())
		db, err := config.Config.Db.Connect()
		if err != nil {
			return fmt.Errorf("could not connect to db: %w", err)
		}
		c := httpclient.NewHttpClient(clientTimeout)
		store := model.NewStore(db)
		go func() {
			if err := emailqueue.Run(c, store); err != nil {
				log.Fatal("email queue error", err)
			}
		}()
		return server.Serve(c, store)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
