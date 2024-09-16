package cmd

import (
	"math/rand"
	"os"
	"time"

	"github.com/spf13/cobra"
	server "github.com/xr0-org/progstack/internal/app"
)

var rootCmd = &cobra.Command{
	Use:   "progstack.com",
	Short: "Run progstack.com",
	Run: func(cmd *cobra.Command, args []string) {
		rand.Seed(time.Now().UnixNano())
		server.Serve()
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
