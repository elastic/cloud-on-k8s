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

	"github.com/elastic/cloud-on-k8s/pkg/about"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	controllerscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/pkg/dev"
	"github.com/elastic/cloud-on-k8s/pkg/dev/portforward"
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"

	// todo (sabo)
	// "github.com/elastic/cloud-on-k8s/pkg/webhook"

	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/kubernetes"
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
		"localhost:6060",
		"Listen address for debug HTTP server (only available in development mode)",
	)

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
		log.Info("Starting debug HTTP server", "addr", pprofServer.Addr)

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

	operatorNamespace := viper.GetString(OperatorNamespaceFlag)
	if operatorNamespace == "" {
		log.Error(fmt.Errorf("%s is a required flag", OperatorNamespaceFlag),
			"required configuration missing")
		os.Exit(1)
	}

	// Get a config to talk to the apiserver
	log.Info("Setting up client for manager")
	cfg := ctrl.GetConfigOrDie()
	// Setup Scheme for all resources
	log.Info("Setting up scheme")
	_ = controllerscheme.SetupScheme()

	// Create a new Cmd to provide shared dependencies and start components
	log.Info("Setting up manager")
	opts := ctrl.Options{
		Scheme: clientgoscheme.Scheme,
		// restrict the operator to watch resources within a single namespace, unless empty
		Namespace: viper.GetString(NamespaceFlagName),
	}

	// only expose prometheus metrics if provided a specific port
	metricsPort := viper.GetInt(MetricsPortFlag)
	if metricsPort != 0 {
		log.Info("Exposing Prometheus metrics on /metrics", "port", metricsPort)
		opts.MetricsBindAddress = fmt.Sprintf(":%d", metricsPort)
	}

	mgr, err := ctrl.NewManager(cfg, opts)
	if err != nil {
		log.Error(err, "unable to create controller manager")
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

	// Setup a client to set the operator uuid config map
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "unable to create k8s clientset")
		os.Exit(1)
	}

	operatorInfo, err := about.GetOperatorInfo(clientset, operatorNamespace, roles)
	if err != nil {
		log.Error(err, "unable to get operator info")
		os.Exit(1)
	}
	log.Info("Setting up controllers", "roles", roles)
	params := operator.Parameters{
		Dialer:            dialer,
		OperatorNamespace: operatorNamespace,
		OperatorInfo:      operatorInfo,
		CACertRotation: certificates.RotationParams{
			Validity:     caCertValidity,
			RotateBefore: caCertRotateBefore,
		},
		CertRotation: certificates.RotationParams{
			Validity:     certValidity,
			RotateBefore: certRotateBefore,
		},
	}

	if err = (apmserver.NewReconciler(mgr, params)).SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to create controller", "controller", "ApmSErver")
		os.Exit(1)
	}
	if err = (elasticsearch.NewReconciler(mgr, params)).SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to create controller", "controller", "Elasticsearch")
		os.Exit(1)
	}
	if err = (kibana.NewReconciler(mgr, params)).SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to create controller", "controller", "Kibana")
		os.Exit(1)
	}

	// need to set up these Add funcs
	// "github.com/elastic/cloud-on-k8s/pkg/controller/apmserverelasticsearchassociation"
	// "github.com/elastic/cloud-on-k8s/pkg/controller/kibanaassociation"
	// 	"github.com/elastic/cloud-on-k8s/pkg/controller/license"
	// "github.com/elastic/cloud-on-k8s/pkg/controller/license/trial"

	// todo sabo
	// log.Info("Setting up webhooks")
	// if err := webhook.AddToManager(mgr, roles, newWebhookParameters); err != nil {
	// 	log.Error(err, "unable to register webhooks to the manager")
	// 	os.Exit(1)
	// }

	log.Info("Starting the manager", "uuid", operatorInfo.OperatorUUID,
		"namespace", operatorNamespace, "version", operatorInfo.BuildInfo.Version,
		"build_hash", operatorInfo.BuildInfo.Hash, "build_date", operatorInfo.BuildInfo.Date,
		"build_snapshot", operatorInfo.BuildInfo.Snapshot)
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "unable to run the manager")
		os.Exit(1)
	}
}

// todo sabo
// func newWebhookParameters() (*webhook.Parameters, error) {
// 	autoInstall := viper.GetBool(AutoInstallWebhooksFlag)
// 	ns := viper.GetString(OperatorNamespaceFlag)
// 	if ns == "" && autoInstall {
// 		return nil, fmt.Errorf("%s needs to be set for webhook auto installation", OperatorNamespaceFlag)
// 	}
// 	svcSelector := viper.GetString(WebhookPodsLabelFlag)
// 	sec := viper.GetString(WebhookSecretFlag)
// 	return &webhook.Parameters{
// 		Bootstrap: webhook.NewBootstrapOptions(webhook.BootstrapOptionsParams{
// 			Namespace:        ns,
// 			ManagedNamespace: viper.GetString(NamespaceFlagName),
// 			SecretName:       sec,
// 			ServiceSelector:  svcSelector,
// 		}),
// 		AutoInstall: autoInstall,
// 	}, nil
// }

func ValidateCertExpirationFlags(validityFlag string, rotateBeforeFlag string) (time.Duration, time.Duration) {
	certValidity := viper.GetDuration(validityFlag)
	certRotateBefore := viper.GetDuration(rotateBeforeFlag)
	if certRotateBefore > certValidity {
		log.Error(fmt.Errorf("%s must be larger than %s", validityFlag, rotateBeforeFlag), "")
		os.Exit(1)
	}
	return certValidity, certRotateBefore
}
