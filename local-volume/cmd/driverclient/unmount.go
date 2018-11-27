package main

import (
	"fmt"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/client"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(unmountCmd)
}

var unmountCmd = &cobra.Command{
	Use:  "unmount",
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(
			client.Unmount(client.NewCaller(), args),
		)
	},
}
