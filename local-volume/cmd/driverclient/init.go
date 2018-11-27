package main

import (
	"fmt"
	"net/http"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/client"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use: "init",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(client.Init(client.NewCaller(&http.Client{})))
	},
}
