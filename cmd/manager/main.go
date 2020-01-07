// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package manager

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"strings"
	"time"

	// allow gcp authentication
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	esv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver"
	asesassn "github.com/elastic/cloud-on-k8s/pkg/controller/apmserverelasticsearchassociation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	controllerscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	kbassn "github.com/elastic/cloud-on-k8s/pkg/controller/kibanaassociation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/license"
	licensetrial "github.com/elastic/cloud-on-k8s/pkg/controller/license/trial"
	"github.com/elastic/cloud-on-k8s/pkg/controller/webhook"
	"github.com/elastic/cloud-on-k8s/pkg/dev"
	"github.com/elastic/cloud-on-k8s/pkg/dev/portforward"
	licensing "github.com/elastic/cloud-on-k8s/pkg/license"
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	MetricsPortFlag   = "metrics-port"
	DefaultMetricPort = 0 // disabled

	AutoPortForwardFlagName = "auto-port-forward"
	NamespacesFlag          = "namespaces"

	CACertValidityFlag     = "ca-cert-validity"
	CACertRotateBeforeFlag = "ca-cert-rotate-before"
	CertValidityFlag       = "cert-validity"
	CertRotateBeforeFlag   = "cert-rotate-before"

	OperatorNamespaceFlag = "operator-namespace"

	ManageWebhookCertsFlag   = "manage-webhook-certs"
	WebhookSecretFlag        = "webhook-secret"
	WebhookConfigurationName = "elastic-webhook.k8s.elastic.co"
	WebhookPort              = 9443

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

	Cmd.Flags().StringSlice(
		NamespacesFlag,
		nil,
		"comma-separated list of namespaces in which this operator should manage resources (defaults to all namespaces)",
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
		ManageWebhookCertsFlag,
		true,
		"enables automatic certificates management for the webhook. The Secret and the ValidatingWebhookConfiguration must be created before running the operator",
	)
	Cmd.Flags().String(
		OperatorNamespaceFlag,
		"",
		"k8s namespace the operator runs in",
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
	if err := controllerscheme.SetupScheme(); err != nil {
		log.Error(err, "Error setting up scheme")
		os.Exit(1)
	}
	// also set up the v1beta1 scheme, used by the v1beta1 webhook
	if err := controllerscheme.SetupV1beta1Scheme(); err != nil {
		log.Error(err, "Error setting up v1beta1 schemes")
		os.Exit(1)
	}

	// Create a new Cmd to provide shared dependencies and start components
	log.Info("Setting up manager")
	opts := ctrl.Options{
		Scheme: clientgoscheme.Scheme,
	}

	// configure the manager cache based on the number of managed namespaces
	managedNamespaces := viper.GetStringSlice(NamespacesFlag)
	switch len(managedNamespaces) {
	case 0:
		log.Info("Operator configured to manage all namespaces")
	case 1:
		log.Info("Operator configured to manage a single namespace", "namespace", managedNamespaces[0])
		opts.Namespace = managedNamespaces[0]
	default:
		log.Info("Operator configured to manage multiple namespaces", "namespaces", managedNamespaces)
		opts.NewCache = cache.MultiNamespacedCacheBuilder(managedNamespaces)
	}

	// only expose prometheus metrics if provided a non-zero port
	metricsPort := viper.GetInt(MetricsPortFlag)
	if metricsPort != 0 {
		log.Info("Exposing Prometheus metrics on /metrics", "port", metricsPort)
	}
	opts.MetricsBindAddress = fmt.Sprintf(":%d", metricsPort) // 0 to disable

	opts.Port = WebhookPort
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

	if operator.HasRole(operator.WebhookServer, roles) {
		setupWebhook(mgr, params.CertRotation, clientset)
	}

	if operator.HasRole(operator.NamespaceOperator, roles) {
		if err = apmserver.Add(mgr, params); err != nil {
			log.Error(err, "unable to create controller", "controller", "ApmServer")
			os.Exit(1)
		}
		if err = elasticsearch.Add(mgr, params); err != nil {
			log.Error(err, "unable to create controller", "controller", "Elasticsearch")
			os.Exit(1)
		}
		if err = kibana.Add(mgr, params); err != nil {
			log.Error(err, "unable to create controller", "controller", "Kibana")
			os.Exit(1)
		}
		if err = asesassn.Add(mgr, params); err != nil {
			log.Error(err, "unable to create controller", "controller", "ApmServerElasticsearchAssociation")
			os.Exit(1)
		}
		if err = kbassn.Add(mgr, params); err != nil {
			log.Error(err, "unable to create controller", "controller", "KibanaAssociation")
			os.Exit(1)
		}

		// Garbage collect any orphaned user Secrets leftover from deleted resources while the operator was not running.
		garbageCollectUsers(cfg, managedNamespaces)
	}
	if operator.HasRole(operator.GlobalOperator, roles) {
		if err = license.Add(mgr, params); err != nil {
			log.Error(err, "unable to create controller", "controller", "License")
			os.Exit(1)
		}
		if err = licensetrial.Add(mgr, params); err != nil {
			log.Error(err, "unable to create controller", "controller", "LicenseTrial")
			os.Exit(1)
		}

		go func() {
			time.Sleep(10 * time.Second)         // wait some arbitrary time for the manager to start
			mgr.GetCache().WaitForCacheSync(nil) // wait until k8s client cache is initialized
			r := licensing.NewResourceReporter(mgr.GetClient())
			r.Start(operatorNamespace, licensing.ResourceReporterFrequency)
		}()
	}
	log.Info("Starting the manager", "uuid", operatorInfo.OperatorUUID,
		"namespace", operatorNamespace, "version", operatorInfo.BuildInfo.Version,
		"build_hash", operatorInfo.BuildInfo.Hash, "build_date", operatorInfo.BuildInfo.Date,
		"build_snapshot", operatorInfo.BuildInfo.Snapshot)
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "unable to run the manager")
		os.Exit(1)
	}
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

func garbageCollectUsers(cfg *rest.Config, managedNamespaces []string) {
	ugc, err := association.NewUsersGarbageCollector(cfg, managedNamespaces)
	if err != nil {
		log.Error(err, "user garbage collector creation failed")
		os.Exit(1)
	}
	err = ugc.
		For(&apmv1.ApmServerList{}, asesassn.AssociationLabelNamespace, asesassn.AssociationLabelName).
		For(&kbv1.KibanaList{}, kbassn.AssociationLabelNamespace, kbassn.AssociationLabelName).
		DoGarbageCollection()
	if err != nil {
		log.Error(err, "user garbage collector failed")
		os.Exit(1)
	}
}

func setupWebhook(mgr manager.Manager, certRotation certificates.RotationParams, clientset kubernetes.Interface) {
	manageWebhookCerts := viper.GetBool(ManageWebhookCertsFlag)
	if manageWebhookCerts {
		log.Info("Automatic management of the webhook certificates enabled")
		// Ensure that all the certificates needed by the webhook server are already created
		webhookParams := webhook.Params{
			Namespace:                viper.GetString(OperatorNamespaceFlag),
			SecretName:               viper.GetString(WebhookSecretFlag),
			WebhookConfigurationName: WebhookConfigurationName,
			Rotation:                 certRotation,
		}

		// Force a first reconciliation to create the resources before the server is started
		if err := webhookParams.ReconcileResources(clientset); err != nil {
			log.Error(err, "unable to setup and fill the webhook certificates")
			os.Exit(1)
		}

		if err := webhook.Add(mgr, webhookParams, clientset); err != nil {
			log.Error(err, "unable to create controller", "controller", webhook.ControllerName)
			os.Exit(1)
		}
	}

	// setup v1 and v1beta1 webhooks
	if err := (&esv1.Elasticsearch{}).SetupWebhookWithManager(mgr); err != nil {
		log.Error(err, "unable to create webhook", "version", "v1", "webhook", "Elasticsearch")
		os.Exit(1)
	}
	if err := (&esv1beta1.Elasticsearch{}).SetupWebhookWithManager(mgr); err != nil {
		log.Error(err, "unable to create webhook", "version", "v1beta1", "webhook", "Elasticsearch")
		os.Exit(1)
	}

	// wait for the secret to be populated in the local filesystem before returning
	interval := time.Second * 1
	timeout := time.Second * 30
	keyPath := filepath.Join(mgr.GetWebhookServer().CertDir, certificates.CertFileName)
	log.Info("Polling for the webhook certificate to be available", "path", keyPath)
	err := wait.PollImmediate(interval, timeout, func() (bool, error) {
		_, err := os.Stat(keyPath)
		// err could be that the file does not exist, but also that permission was denied or something else
		if os.IsNotExist(err) {
			log.V(1).Info("Webhook certificate file not present on filesystem yet", "path", keyPath)
			return false, nil
		} else if err != nil {
			log.Error(err, "Error checking if webhook secret path exists", "path", keyPath)
			return false, err
		}
		log.V(1).Info("Webhook certificate file present on filesystem", "path", keyPath)
		return true, nil
	})

	if err != nil {
		os.Exit(1)
	}
}
