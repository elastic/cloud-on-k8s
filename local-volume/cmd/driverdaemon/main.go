package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/drivers"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/drivers/lvm"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	driverKindFlag     = "driver-kind"
	lvmVolumeGroupFlag = "lvm-volume-group"
)

var rootCmd = &cobra.Command{
	Short: "Run the local volume driver daemon",
	Run: func(cmd *cobra.Command, args []string) {
		driverKind := viper.GetString(driverKindFlag)
		driverOpts := drivers.Options{
			LVM: lvm.Options{
				VolumeGroupName: viper.GetString(lvmVolumeGroupFlag),
			},
		}
		log.Fatal(daemon.Start(driverKind, driverOpts))
	},
}

func main() {
	flags := rootCmd.Flags()

	flags.String(driverKindFlag, lvm.DriverKind, "Driver kind (eg. LVM or BINDMOUNT)")
	flags.String(lvmVolumeGroupFlag, lvm.DefaultVolumeGroup, "LVM Volume Group to be used for provisioning logical volumes")

	// Bind flags to environment variables
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
	viper.BindPFlags(rootCmd.Flags())

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
