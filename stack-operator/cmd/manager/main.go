package manager

import (
	"fmt"
	"os"
	"strings"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/snapshot"
	"github.com/elastic/stack-operators/stack-operator/pkg/dev/portforward"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/net"
	"github.com/elastic/stack-operators/stack-operator/pkg/webhook"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

const (
	MetricsPortFlag   = "metrics-port"
	DefaultMetricPort = 8080

	AutoPortForwardFlagName = "auto-port-forward"
)

var (
	// development is whether we should be in development mode or not (affects logging and development-specific features)
	development = false

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

	log = logf.Log.WithName("manager")
)

func init() {
	// development mode is only available as a command line flag to avoid accidentally enabling it
	Cmd.Flags().BoolVar(&development, "development", false, "turns on development mode")

	Cmd.Flags().String(
		snapshot.SnapshotterImageFlag,
		"",
		"image to use for the snapshotter application",
	)
	Cmd.Flags().Bool(
		AutoPortForwardFlagName,
		false,
		"enables automatic port-forwarding "+
			"(for dev use only as it exposes k8s resources on ephemeral ports to localhost)",
	)
	Cmd.Flags().Int(
		MetricsPortFlag,
		DefaultMetricPort,
		"Port to use for exposing metrics in the Prometheus format (set 0 to disable)",
	)

	viper.BindPFlags(Cmd.Flags())
	// enable using dashed notation in flags and underscores in env
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	if err := viper.BindPFlags(Cmd.Flags()); err != nil {
		log.Error(err, "Unexpected error while binding flags")
		os.Exit(1)
	}

	viper.AutomaticEnv()
}

func execute() {
	logf.SetLogger(logf.ZapLogger(development))

	log := logf.Log.WithName("manager")

	var dialer net.Dialer
	autoPortForward := viper.GetBool(AutoPortForwardFlagName)
	if !development && autoPortForward {
		panic(fmt.Sprintf(
			"Enabling %s without enabling development mode not allowed", AutoPortForwardFlagName,
		))
	} else if autoPortForward {
		log.Info("Warning: auto-port-forwarding is enabled, which is intended for development only")
		dialer = portforward.NewForwardingDialer()
	}

	if viper.GetString(snapshot.SnapshotterImageFlag) == "" {
		log.Error(fmt.Errorf("%s is a required flag", snapshot.SnapshotterImageFlag),
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
	opts := manager.Options{}
	metricsPort := viper.GetInt(MetricsPortFlag)
	if metricsPort != 0 {
		opts.MetricsBindAddress = fmt.Sprintf(":%d", metricsPort)
		log.Info(fmt.Sprintf("Exposing Prometheus metrics on /metrics%s", opts.MetricsBindAddress))
	}
	mgr, err := manager.New(cfg, opts)
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
	if err := controller.AddToManager(mgr, dialer); err != nil {
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
