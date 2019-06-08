// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package manager

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/dev"
	"github.com/elastic/cloud-on-k8s/operators/pkg/dev/portforward"
	"github.com/elastic/cloud-on-k8s/operators/pkg/info"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/net"
	"github.com/elastic/cloud-on-k8s/operators/pkg/webhook"
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
	NamespaceFlagName       = "namespace"

	CACertValidityFlag     = "ca-cert-validity"
	CACertRotateBeforeFlag = "ca-cert-rotate-before"
	CertValidityFlag       = "cert-validity"
	CertRotateBeforeFlag   = "cert-rotate-before"

	AutoInstallWebhooksFlag = "auto-install-webhooks"
	OperatorNamespaceFlag   = "operator-namespace"
	WebhookSecretFlag       = "webhook-secret"
	WebhookPodsLabelFlag    = "webhook-pods-label"

	DebugHTTPServerListenAddressFlag = "debug-http-listen"
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

	log = logf.Log.WithName("manager")
)

func init() {

	Cmd.Flags().String(
		NamespaceFlagName,
		"",
		"namespace in which this operator should manage resources (defaults to all namespaces)",
	)
	Cmd.Flags().String(
		operator.ImageFlag,
		"",
		"image containing the binaries for this operator",
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
	Cmd.Flags().StringSlice(
		operator.RoleFlag,
		[]string{operator.All},
		"Roles this operator should assume (either namespace, global, webhook or all)",
	)
	Cmd.Flags().Duration(
		CACertValidityFlag,
		certificates.DefaultCertValidity,
		"Duration representing how long before a newly created CA cert expires",
	)
	Cmd.Flags().Duration(
		CACertRotateBeforeFlag,
		certificates.DefaultRotateBefore,
		"Duration representing how long before expiration CA certificates should be reissued",
	)
	Cmd.Flags().Duration(
		CertValidityFlag,
		certificates.DefaultCertValidity,
		"Duration representing how long before a newly created TLS certificate expires",
	)
	Cmd.Flags().Duration(
		CertRotateBeforeFlag,
		certificates.DefaultRotateBefore,
		"Duration representing how long before expiration TLS certificates should be reissued",
	)
	Cmd.Flags().Bool(
		AutoInstallWebhooksFlag,
		true,
		"enables automatic webhook installation (RBAC permission for service, secret and validatingwebhookconfigurations needed)",
	)
	Cmd.Flags().String(
		OperatorNamespaceFlag,
		"",
		"k8s namespace the operator runs in",
	)
	Cmd.Flags().String(
		WebhookPodsLabelFlag,
		"",
		"k8s label to select pods running the operator",
	)
	Cmd.Flags().String(
		WebhookSecretFlag,
		"",
		"k8s secret mounted into /tmp/cert to be used for webhook certificates",
	)
	Cmd.Flags().String(
		DebugHTTPServerListenAddressFlag,
		":6060",
		"Listen address for debug HTTP server (only available in development mode)",
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
	if dev.Enabled {
		// expose pprof if development mode is enabled
		mux := http.NewServeMux()
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

		pprofServer := http.Server{
			Addr:    viper.GetString(DebugHTTPServerListenAddressFlag),
			Handler: mux,
		}
		log.Info("Starting Debug HTTP Server.", "addr", pprofServer.Addr)

		go func() {
			err := pprofServer.ListenAndServe()
			panic(err)
		}()
	}

	var dialer net.Dialer
	autoPortForward := viper.GetBool(AutoPortForwardFlagName)
	if !dev.Enabled && autoPortForward {
		panic(fmt.Sprintf(
			"Enabling %s without enabling development mode not allowed", AutoPortForwardFlagName,
		))
	} else if autoPortForward {
		log.Info("Warning: auto-port-forwarding is enabled, which is intended for development only")
		dialer = portforward.NewForwardingDialer()
	}

	operatorImage := viper.GetString(operator.ImageFlag)
	if operatorImage == "" {
		log.Error(fmt.Errorf("%s is a required flag", operator.ImageFlag),
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
	opts := manager.Options{
		// restrict the operator to watch resources within a single namespace, unless empty
		Namespace: viper.GetString(NamespaceFlagName),
	}
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

	// Verify cert validity options
	caCertValidity, caCertRotateBefore := ValidateCertExpirationFlags(CACertValidityFlag, CACertRotateBeforeFlag)
	certValidity, certRotateBefore := ValidateCertExpirationFlags(CertValidityFlag, CertRotateBeforeFlag)
	// Setup all Controllers
	roles := viper.GetStringSlice(operator.RoleFlag)
	err = operator.ValidateRoles(roles)
	if err != nil {
		log.Error(err, "invalid roles specified")
		os.Exit(1)
	}

	operatorNamespace := viper.GetString(OperatorNamespaceFlag)
	operatorInfo := info.New(operatorNamespace, cfg)

	log.Info("Setting up controller", "roles", roles)
	if err := controller.AddToManager(mgr, roles, operator.Parameters{
		Dialer:             dialer,
		OperatorImage:      operatorImage,
		OperatorNamespace:  operatorNamespace,
		OperatorInfo:       operatorInfo,
		CACertValidity:     caCertValidity,
		CACertRotateBefore: caCertRotateBefore,
		CertValidity:       certValidity,
		CertRotateBefore:   certRotateBefore,
	}); err != nil {
		log.Error(err, "unable to register controllers to the manager")
		os.Exit(1)
	}

	log.Info("setting up webhooks")
	if err := webhook.AddToManager(mgr, roles, newWebhookParameters); err != nil {
		log.Error(err, "unable to register webhooks to the manager")
		os.Exit(1)
	}

	log.Info("Starting the manager",
		"namespace", operatorInfo.Namespace, "version", operatorInfo.Version.Number,
		"build_hash", operatorInfo.Version.BuildHash, "build_date", operatorInfo.Version.BuildDate,
		"build_snapshot", operatorInfo.Version.BuildSnapshot)
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "unable to run the manager")
		os.Exit(1)
	}
}

func newWebhookParameters() (*webhook.Parameters, error) {
	autoInstall := viper.GetBool(AutoInstallWebhooksFlag)
	ns := viper.GetString(OperatorNamespaceFlag)
	if ns == "" && autoInstall {
		return nil, fmt.Errorf("%s needs to be set for webhook auto installation", OperatorNamespaceFlag)
	}
	svcSelector := viper.GetString(WebhookPodsLabelFlag)
	sec := viper.GetString(WebhookSecretFlag)
	return &webhook.Parameters{
		Bootstrap: webhook.NewBootstrapOptions(webhook.BootstrapOptionsParams{
			Namespace:        ns,
			ManagedNamespace: viper.GetString(NamespaceFlagName),
			SecretName:       sec,
			ServiceSelector:  svcSelector,
		}),
		AutoInstall: autoInstall,
	}, nil
}

func ValidateCertExpirationFlags(validityFlag string, rotateBeforeFlag string) (time.Duration, time.Duration) {
	certValidity := viper.GetDuration(validityFlag)
	certRotateBefore := viper.GetDuration(rotateBeforeFlag)
	if certRotateBefore > certValidity {
		log.Error(fmt.Errorf("%s must be larger than %s", validityFlag, rotateBeforeFlag), "")
		os.Exit(1)
	}
	return certValidity, certRotateBefore
}
