package main

import (
	"fmt"
	"net/http"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/client"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(mountCmd)
}

var mountCmd = &cobra.Command{
	Use:  "mount",
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(
			client.Mount(client.NewCaller(&http.Client{}), args),
		)
	},
}
