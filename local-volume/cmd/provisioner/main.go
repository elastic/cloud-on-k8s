package main

import (
	"fmt"
	"os"

	"github.com/elastic/stack-operators/local-volume/pkg/provisioner"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Short: "Run the local volume provisioner",
	Run: func(cmd *cobra.Command, args []string) {
		err := provisioner.Start()
		if err != nil {
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
