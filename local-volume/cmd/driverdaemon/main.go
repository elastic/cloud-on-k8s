package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/cmdutil"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/drivers"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/drivers/bindmount"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/drivers/lvm"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	driverKindFlag = "driver-kind"

	lvmVolumeGroupFlag    = "lvm-volume-group"
	lvmUseThinVolumesFlag = "lvm-use-thin-volumes"
	lvmThinPoolFlag       = "lvm-thin-pool"
	nodeNameFlag          = "node-name"
)

var rootCmd = &cobra.Command{
	Short: "Run the local volume driver daemon",
	Run: func(cmd *cobra.Command, args []string) {
		nodeName := viper.GetString(nodeNameFlag)
		if nodeName == "" {
			log.Fatal("$NODE_NAME should be set by referencing spec.nodeName")
		}
		driverKind := viper.GetString(driverKindFlag)
		driverOpts := drivers.Options{
			BindMount: bindmount.Options{
				Factory:   cmdutil.NewExecutableFactory(),
				MountPath: bindmount.DefaultContainerMountPath,
			},
			LVM: lvm.Options{
				ExecutableFactory: cmdutil.NewExecutableFactory(),
				VolumeGroupName:   viper.GetString(lvmVolumeGroupFlag),
				UseThinVolumes:    viper.GetBool(lvmUseThinVolumesFlag),
				ThinPoolName:      viper.GetString(lvmThinPoolFlag),
			},
		}

		server, err := daemon.NewServer(nodeName, driverKind, driverOpts)
		if err != nil {
			log.WithError(err).Fatal("Cannot create driver daemon")
		}

		log.Fatal(server.Start())
	},
}

func main() {
	flags := rootCmd.Flags()

	// Driver kind
	flags.String(driverKindFlag, lvm.DriverKind, "Driver kind (eg. LVM or BINDMOUNT)")

	// LVM flags
	flags.String(lvmVolumeGroupFlag, lvm.DefaultVolumeGroup, "LVM Volume Group to be used for provisioning logical volumes")
	flags.Bool(lvmUseThinVolumesFlag, lvm.DefaultUseThinVolumes, "Use LVM thin volumes")
	flags.String(lvmThinPoolFlag, lvm.DefaultThinPoolName, "LVM thin pool name")
	// node name should be passed to the environment through the yaml spec
	flags.String(nodeNameFlag, "", "Name of the node this pod is running on")

	// Bind flags to environment variables
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
	viper.BindPFlags(flags)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
