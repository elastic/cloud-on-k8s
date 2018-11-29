package manager

import (
	"fmt"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch"
	"os"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller"
	"github.com/elastic/stack-operators/stack-operator/pkg/webhook"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

var (
	// Cmd is the cobra command to start the manager.
	Cmd = &cobra.Command{
		Use:   "manager",
		Short: "Start the operator manager",
		Long: `manager starts the manager for this operator,
 which will in turn create the necessary controller.`,
		Run: func(cmd *cobra.Command, args []string) {
			execute()
		},
	}
)

func init() {
	Cmd.Flags().StringP(elasticsearch.SnapshotterImageFlag, "s", "", "image to use for the snappshotter application")
	viper.BindPFlags(Cmd.Flags())
	viper.AutomaticEnv()
}

func execute() {
	log := logf.Log.WithName("manager")

	if viper.GetString(elasticsearch.SnapshotterImageFlag) == "" {
		log.Error(fmt.Errorf("%s is a required flag", elasticsearch.SnapshotterImageFlag),
			"required configuration missing")
		os.Exit(1)
	}

	// Get a config to talk to the apiserver
	log.Info("setting up client for manager")
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "unable to set up client config")
		os.Exit(1)
	}

	// Create a new Cmd to provide shared dependencies and start components
	log.Info("setting up manager")
	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		log.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	log.Info("setting up scheme")
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "unable add APIs to scheme")
		os.Exit(1)
	}

	// Setup all Controllers
	log.Info("Setting up controller")
	if err := controller.AddToManager(mgr); err != nil {
		log.Error(err, "unable to register controllers to the manager")
		os.Exit(1)
	}

	log.Info("setting up webhooks")
	if err := webhook.AddToManager(mgr); err != nil {
		log.Error(err, "unable to register webhooks to the manager")
		os.Exit(1)
	}

	// Start the Cmd
	log.Info("Starting the Cmd.")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "unable to run the manager")
		os.Exit(1)
	}
}
