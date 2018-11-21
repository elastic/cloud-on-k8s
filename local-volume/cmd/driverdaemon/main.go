package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/bindmount"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const driverKindFlag = "driver-kind"

var rootCmd = &cobra.Command{
	Short: "Run the local volume driver daemon",
	Run: func(cmd *cobra.Command, args []string) {
		log.Fatal(daemon.Start(viper.GetString(driverKindFlag)))
	},
}

func main() {
	flags := rootCmd.Flags()
	flags.String(driverKindFlag, bindmount.DriverKind, "Driver kind (eg. LVM or BINDMOUNT)")
	// Also match $DRIVER_KIND
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.BindPFlag(driverKindFlag, flags.Lookup(driverKindFlag))
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
