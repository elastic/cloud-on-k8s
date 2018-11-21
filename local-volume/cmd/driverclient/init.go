package main

import (
	"fmt"

	"github.com/elastic/localvolume/pkg/driver/client"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use: "init",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(client.Init())
	},
}
