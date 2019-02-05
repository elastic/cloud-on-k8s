package main

import (
	"fmt"
	"os"

	"github.com/elastic/k8s-operators/local-volume/pkg/provisioner"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Short: "Run the local volume provisioner",
	Run: func(cmd *cobra.Command, args []string) {
		if err := provisioner.Start(); err != nil {
			log.Error(err)
		}
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
