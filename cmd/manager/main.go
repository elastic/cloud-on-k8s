// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package manager

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	apmv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1beta1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	esv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	entv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	kbv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	associationctl "github.com/elastic/cloud-on-k8s/pkg/controller/association/controller"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	controllerscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch"
	"github.com/elastic/cloud-on-k8s/pkg/controller/enterprisesearch"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/pkg/controller/license"
	licensetrial "github.com/elastic/cloud-on-k8s/pkg/controller/license/trial"
	"github.com/elastic/cloud-on-k8s/pkg/controller/remoteca"
	"github.com/elastic/cloud-on-k8s/pkg/controller/webhook"
	"github.com/elastic/cloud-on-k8s/pkg/dev"
	"github.com/elastic/cloud-on-k8s/pkg/dev/portforward"
	licensing "github.com/elastic/cloud-on-k8s/pkg/license"
	logconf "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.elastic.co/apm"
	"go.uber.org/automaxprocs/maxprocs"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // allow gcp authentication
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

const (
	DefaultMetricPort  = 0 // disabled
	DefaultWebhookName = "elastic-webhook.k8s.elastic.co"
	WebhookPort        = 9443

	LeaderElectionConfigMapName = "elastic-operator-leader"

	debugHTTPShutdownTimeout = 5 * time.Second // time to allow for the debug HTTP server to shutdown
)

var (
	configFile string
	log        logr.Logger
)

func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manager",
		Short: "Start the Elastic Cloud on Kubernetes operator",
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			// enable using dashed notation in flags and underscores in env
			viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

			if err := viper.BindPFlags(cmd.Flags()); err != nil {
				return fmt.Errorf("failed to bind flags: %w", err)
			}

			viper.AutomaticEnv()

			if configFile != "" {
				viper.SetConfigFile(configFile)
				if err := viper.ReadInConfig(); err != nil {
					return fmt.Errorf("failed to read config file %s: %w", configFile, err)
				}

				if !viper.GetBool(operator.DisableConfigWatch) {
					viper.WatchConfig()
				}
			}

			logconf.ChangeVerbosity(viper.GetInt(logconf.FlagName))
			log = logf.Log.WithName("manager")

			return nil
		},
		RunE: doRun,
	}

	cmd.Flags().Bool(
		operator.AutoPortForwardFlag,
		false,
		"Enables automatic port-forwarding "+
			"(for dev use only as it exposes k8s resources on ephemeral ports to localhost)",
	)
	cmd.Flags().Duration(
		operator.CACertRotateBeforeFlag,
		certificates.DefaultRotateBefore,
		"Duration representing how long before expiration CA certificates should be reissued",
	)
	cmd.Flags().Duration(
		operator.CACertValidityFlag,
		certificates.DefaultCertValidity,
		"Duration representing how long before a newly created CA cert expires",
	)
	cmd.Flags().Duration(
		operator.CertRotateBeforeFlag,
		certificates.DefaultRotateBefore,
		"Duration representing how long before expiration TLS certificates should be reissued",
	)
	cmd.Flags().Duration(
		operator.CertValidityFlag,
		certificates.DefaultCertValidity,
		"Duration representing how long before a newly created TLS certificate expires",
	)
	cmd.Flags().StringVar(
		&configFile,
		operator.ConfigFlag,
		"",
		"Path to the file containing the operator configuration",
	)
	cmd.Flags().String(
		operator.ContainerRegistryFlag,
		container.DefaultContainerRegistry,
		"Container registry to use when downloading Elastic Stack container images",
	)
	cmd.Flags().String(
		operator.ContainerSuffixFlag,
		"",
		"Container image suffix to use when downloading Elastic Stack container images",
	)
	cmd.Flags().String(
		operator.DebugHTTPListenFlag,
		"localhost:6060",
		"Listen address for debug HTTP server (only available in development mode)",
	)
	cmd.Flags().Bool(
		operator.DisableConfigWatch,
		false,
		"Disable watching the configuration file for changes",
	)
	cmd.Flags().Bool(
		operator.EnforceRBACOnRefsFlag,
		false, // Set to false for backward compatibility
		"Restrict cross-namespace resource association through RBAC (eg. referencing Elasticsearch from Kibana)",
	)
	cmd.Flags().Bool(
		operator.EnableLeaderElection,
		true,
		"Enable leader election. Enabling this will ensure there is only one active operator.",
	)
	cmd.Flags().Bool(
		operator.EnableTracingFlag,
		false,
		"Enable APM tracing in the operator. Endpoint, token etc are to be configured via environment variables. See https://www.elastic.co/guide/en/apm/agent/go/1.x/configuration.html")
	cmd.Flags().Bool(
		operator.EnableWebhookFlag,
		false,
		"Enables a validating webhook server in the operator process.",
	)
	cmd.Flags().Bool(
		operator.ManageWebhookCertsFlag,
		true,
		"Enables automatic certificates management for the webhook. The Secret and the ValidatingWebhookConfiguration must be created before running the operator",
	)
	cmd.Flags().Int(
		operator.MaxConcurrentReconcilesFlag,
		3,
		"Sets maximum number of concurrent reconciles per controller (Elasticsearch, Kibana, Apm Server etc). Affects the ability of the operator to process changes concurrently.",
	)
	cmd.Flags().Int(
		operator.MetricsPortFlag,
		DefaultMetricPort,
		"Port to use for exposing metrics in the Prometheus format (set 0 to disable)",
	)
	cmd.Flags().StringSlice(
		operator.NamespacesFlag,
		nil,
		"Comma-separated list of namespaces in which this operator should manage resources (defaults to all namespaces)",
	)
	cmd.Flags().String(
		operator.OperatorNamespaceFlag,
		"",
		"Kubernetes namespace the operator runs in",
	)
	cmd.Flags().String(
		operator.WebhookCertDirFlag,
		// this is controller-runtime's own default, copied here for making the default explicit when using `--help`
		filepath.Join(os.TempDir(), "k8s-webhook-server", "serving-certs"),
		"Path to the directory that contains the webhook server key and certificate",
	)
	cmd.Flags().String(
		operator.WebhookSecretFlag,
		"",
		fmt.Sprintf("Kubernetes secret mounted into the path designated by %s to be used for webhook certificates", operator.WebhookCertDirFlag),
	)
	cmd.Flags().String(
		operator.WebhookNameFlag,
		DefaultWebhookName,
		"Name of the Kubernetes ValidatingWebhookConfiguration resource. Only used when enable-webhook is true.",
	)
	cmd.Flags().Bool(
		operator.SetDefaultSecurityContextFlag,
		true,
		"Enables setting the default security context with fsGroup=1000 for Elasticsearch 8.0+ Pods. Ignored pre-8.0.",
	)

	// hide development mode flags from the usage message
	_ = cmd.Flags().MarkHidden(operator.AutoPortForwardFlag)
	_ = cmd.Flags().MarkHidden(operator.DebugHTTPListenFlag)

	// configure filename auto-completion for the config flag
	_ = cmd.MarkFlagFilename(operator.ConfigFlag)

	logconf.BindFlags(cmd.Flags())

	return cmd
}

func doRun(_ *cobra.Command, _ []string) error {
	signalChan := signals.SetupSignalHandler()
	disableConfigWatch := viper.GetBool(operator.DisableConfigWatch)

	// no config file to watch so start the operator directly
	if configFile == "" || disableConfigWatch {
		return startOperator(signalChan)
	}

	// receive config file update events over a channel
	confUpdateChan := make(chan struct{}, 1)

	viper.OnConfigChange(func(evt fsnotify.Event) {
		if evt.Op&fsnotify.Write == fsnotify.Write || evt.Op&fsnotify.Create == fsnotify.Create {
			confUpdateChan <- struct{}{}
		}
	})

	// start the operator in a goroutine
	errChan := make(chan error, 1)
	stopChan := make(chan struct{})

	go func() {
		err := startOperator(stopChan)
		errChan <- err
	}()

	// watch for events
	for {
		select {
		case err := <-errChan: // operator failed
			log.Error(err, "Shutting down due to error")
			close(stopChan)

			return err
		case <-signalChan: // signal received
			log.Info("Signal received: shutting down")
			close(stopChan)

			return <-errChan
		case <-confUpdateChan: // config file updated
			log.Info("Shutting down to apply updated configuration")
			close(stopChan)

			if err := <-errChan; err != nil {
				log.Error(err, "Encountered error from previous operator run")
				return err
			}

			return nil
		}
	}
}

func startOperator(stopChan <-chan struct{}) error {
	log.V(1).Info("Effective configuration", "values", viper.AllSettings())

	// update GOMAXPROCS to container cpu limit if necessary
	_, err := maxprocs.Set(maxprocs.Logger(func(s string, i ...interface{}) {
		// maxprocs needs an sprintf format string with args, but our logger needs a string with optional key value pairs,
		// so we need to do this translation
		log.Info(fmt.Sprintf(s, i...))
	}))
	if err != nil {
		log.Error(err, "Error setting GOMAXPROCS")
		return err
	}

	if dev.Enabled {
		// expose pprof if development mode is enabled
		mux := http.NewServeMux()
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

		pprofServer := http.Server{
			Addr:    viper.GetString(operator.DebugHTTPListenFlag),
			Handler: mux,
		}
		log.Info("Starting debug HTTP server", "addr", pprofServer.Addr)

		go func() {
			go func() {
				<-stopChan

				ctx, cancelFunc := context.WithTimeout(context.Background(), debugHTTPShutdownTimeout)
				defer cancelFunc()

				if err := pprofServer.Shutdown(ctx); err != nil {
					log.Error(err, "Failed to shutdown debug HTTP server")
				}
			}()

			if err := pprofServer.ListenAndServe(); !errors.Is(http.ErrServerClosed, err) {
				log.Error(err, "Failed to start debug HTTP server")
				panic(err)
			}
		}()
	}

	var dialer net.Dialer
	autoPortForward := viper.GetBool(operator.AutoPortForwardFlag)
	if !dev.Enabled && autoPortForward {
		return fmt.Errorf("development mode must be enabled to use %s", operator.AutoPortForwardFlag)
	} else if autoPortForward {
		log.Info("Warning: auto-port-forwarding is enabled, which is intended for development only")
		dialer = portforward.NewForwardingDialer()
	}

	operatorNamespace := viper.GetString(operator.OperatorNamespaceFlag)
	if operatorNamespace == "" {
		err := fmt.Errorf("operator namespace must be specified using %s", operator.OperatorNamespaceFlag)
		log.Error(err, "Required configuration missing")
		return err
	}

	// set the default container registry
	containerRegistry := viper.GetString(operator.ContainerRegistryFlag)
	log.Info("Setting default container registry", "container_registry", containerRegistry)
	container.SetContainerRegistry(containerRegistry)

	// set a custom container suffix if specified
	containerSuffix := viper.GetString(operator.ContainerSuffixFlag)
	if containerSuffix != "" {
		log.Info("Setting default container suffix", "container_suffix", containerSuffix)
		container.SetContainerSuffix(containerSuffix)
	}

	// Get a config to talk to the apiserver
	cfg, err := ctrl.GetConfig()
	if err != nil {
		log.Error(err, "Failed to obtain client configuration")
		return err
	}

	// Setup Scheme for all resources
	log.Info("Setting up scheme")
	controllerscheme.SetupScheme()
	// also set up the v1beta1 scheme, used by the v1beta1 webhook
	controllerscheme.SetupV1beta1Scheme()

	// Create a new Cmd to provide shared dependencies and start components
	opts := ctrl.Options{
		Scheme:                  clientgoscheme.Scheme,
		CertDir:                 viper.GetString(operator.WebhookCertDirFlag),
		LeaderElection:          viper.GetBool(operator.EnableLeaderElection),
		LeaderElectionID:        LeaderElectionConfigMapName,
		LeaderElectionNamespace: operatorNamespace,
	}

	// configure the manager cache based on the number of managed namespaces
	managedNamespaces := viper.GetStringSlice(operator.NamespacesFlag)
	switch {
	case len(managedNamespaces) == 0:
		log.Info("Operator configured to manage all namespaces")
	case len(managedNamespaces) == 1 && managedNamespaces[0] == operatorNamespace:
		log.Info("Operator configured to manage a single namespace", "namespace", managedNamespaces[0], "operator_namespace", operatorNamespace)
		opts.Namespace = managedNamespaces[0]
	default:
		log.Info("Operator configured to manage multiple namespaces", "namespaces", managedNamespaces, "operator_namespace", operatorNamespace)
		// the manager cache should always include the operator namespace so that we can work with operator-internal resources
		opts.NewCache = cache.MultiNamespacedCacheBuilder(append(managedNamespaces, operatorNamespace))
	}

	// only expose prometheus metrics if provided a non-zero port
	metricsPort := viper.GetInt(operator.MetricsPortFlag)
	if metricsPort != 0 {
		log.Info("Exposing Prometheus metrics on /metrics", "port", metricsPort)
	}
	opts.MetricsBindAddress = fmt.Sprintf(":%d", metricsPort) // 0 to disable

	opts.Port = WebhookPort
	mgr, err := ctrl.NewManager(cfg, opts)
	if err != nil {
		log.Error(err, "Failed to create controller manager")
		return err
	}

	// Verify cert validity options
	caCertValidity, caCertRotateBefore, err := validateCertExpirationFlags(operator.CACertValidityFlag, operator.CACertRotateBeforeFlag)
	if err != nil {
		log.Error(err, "Invalid CA certificate rotation parameters")
		return err
	}

	log.V(1).Info("Using certificate authority rotation parameters", operator.CACertValidityFlag, caCertValidity, operator.CACertRotateBeforeFlag, caCertRotateBefore)

	certValidity, certRotateBefore, err := validateCertExpirationFlags(operator.CertValidityFlag, operator.CertRotateBeforeFlag)
	if err != nil {
		log.Error(err, "Invalid certificate rotation parameters")
		return err
	}

	log.V(1).Info("Using certificate rotation parameters", operator.CertValidityFlag, certValidity, operator.CertRotateBeforeFlag, certRotateBefore)

	// Setup a client to set the operator uuid config map
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "Failed to create Kubernetes client")
		return err
	}

	operatorInfo, err := about.GetOperatorInfo(clientset, operatorNamespace)
	if err != nil {
		log.Error(err, "Failed to get operator info")
		return err
	}

	log.Info("Setting up controllers")
	var tracer *apm.Tracer
	if viper.GetBool(operator.EnableTracingFlag) {
		tracer = tracing.NewTracer("elastic-operator")
	}
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
		MaxConcurrentReconciles:   viper.GetInt(operator.MaxConcurrentReconcilesFlag),
		SetDefaultSecurityContext: viper.GetBool(operator.SetDefaultSecurityContextFlag),
		Tracer:                    tracer,
	}

	if viper.GetBool(operator.EnableWebhookFlag) {
		setupWebhook(mgr, params.CertRotation, clientset)
	}

	enforceRbacOnRefs := viper.GetBool(operator.EnforceRBACOnRefsFlag)

	var accessReviewer rbac.AccessReviewer
	if enforceRbacOnRefs {
		accessReviewer = rbac.NewSubjectAccessReviewer(clientset)
	} else {
		accessReviewer = rbac.NewPermissiveAccessReviewer()
	}

	if err := registerControllers(mgr, params, accessReviewer); err != nil {
		return err
	}

	go asyncTasks(mgr, cfg, managedNamespaces, operatorNamespace)

	log.Info("Starting the manager", "uuid", operatorInfo.OperatorUUID,
		"namespace", operatorNamespace, "version", operatorInfo.BuildInfo.Version,
		"build_hash", operatorInfo.BuildInfo.Hash, "build_date", operatorInfo.BuildInfo.Date,
		"build_snapshot", operatorInfo.BuildInfo.Snapshot)

	if err := mgr.Start(stopChan); err != nil {
		log.Error(err, "Failed to start the controller manager")
		return err
	}

	return nil
}

// asyncTasks schedules some tasks to be started when this instance of the operator is elected
func asyncTasks(mgr manager.Manager, cfg *rest.Config, managedNamespaces []string, operatorNamespace string) {
	<-mgr.Elected() // wait for this operator instance to be elected

	// Start the resource reporter
	go func() {
		time.Sleep(10 * time.Second)         // wait some arbitrary time for the manager to start
		mgr.GetCache().WaitForCacheSync(nil) // wait until k8s client cache is initialized
		r := licensing.NewResourceReporter(mgr.GetClient(), operatorNamespace)
		r.Start(licensing.ResourceReporterFrequency)
	}()

	// Garbage collect any orphaned user Secrets leftover from deleted resources while the operator was not running.
	garbageCollectUsers(cfg, managedNamespaces)
}

func registerControllers(mgr manager.Manager, params operator.Parameters, accessReviewer rbac.AccessReviewer) error {
	controllers := []struct {
		name         string
		registerFunc func(manager.Manager, operator.Parameters) error
	}{
		{name: "APMServer", registerFunc: apmserver.Add},
		{name: "Elasticsearch", registerFunc: elasticsearch.Add},
		{name: "Kibana", registerFunc: kibana.Add},
		{name: "EnterpriseSearch", registerFunc: enterprisesearch.Add},
		{name: "Beats", registerFunc: beat.Add},
		{name: "License", registerFunc: license.Add},
		{name: "LicenseTrial", registerFunc: licensetrial.Add},
	}

	for _, c := range controllers {
		if err := c.registerFunc(mgr, params); err != nil {
			log.Error(err, "Failed to register controller", "controller", c.name)
			return fmt.Errorf("failed to register %s controller: %w", c.name, err)
		}
	}

	assocControllers := []struct {
		name         string
		registerFunc func(manager.Manager, rbac.AccessReviewer, operator.Parameters) error
	}{
		{name: "RemoteCA", registerFunc: remoteca.Add},
		{name: "APM-ES", registerFunc: associationctl.AddApmES},
		{name: "APM-KB", registerFunc: associationctl.AddApmKibana},
		{name: "KB-ES", registerFunc: associationctl.AddKibanaES},
		{name: "ENT-ES", registerFunc: associationctl.AddEntES},
		{name: "BEAT-ES", registerFunc: associationctl.AddBeatES},
		{name: "BEAT-KB", registerFunc: associationctl.AddBeatKibana},
	}

	for _, c := range assocControllers {
		if err := c.registerFunc(mgr, accessReviewer, params); err != nil {
			log.Error(err, "Failed to register association controller", "controller", c.name)
			return fmt.Errorf("failed to register %s association controller: %w", c.name, err)
		}
	}

	return nil
}

func validateCertExpirationFlags(validityFlag string, rotateBeforeFlag string) (time.Duration, time.Duration, error) {
	certValidity := viper.GetDuration(validityFlag)
	certRotateBefore := viper.GetDuration(rotateBeforeFlag)

	if certRotateBefore > certValidity {
		return certValidity, certRotateBefore, fmt.Errorf("%s must be larger than %s", validityFlag, rotateBeforeFlag)
	}

	return certValidity, certRotateBefore, nil
}

func garbageCollectUsers(cfg *rest.Config, managedNamespaces []string) {
	ugc, err := association.NewUsersGarbageCollector(cfg, managedNamespaces)
	if err != nil {
		log.Error(err, "user garbage collector creation failed")
		os.Exit(1)
	}
	err = ugc.
		For(&apmv1.ApmServerList{}, associationctl.ApmAssociationLabelNamespace, associationctl.ApmAssociationLabelName).
		For(&kbv1.KibanaList{}, associationctl.KibanaESAssociationLabelNamespace, associationctl.KibanaESAssociationLabelName).
		For(&entv1beta1.EnterpriseSearchList{}, associationctl.EntESAssociationLabelNamespace, associationctl.EntESAssociationLabelName).
		For(&beatv1beta1.BeatList{}, associationctl.BeatAssociationLabelNamespace, associationctl.BeatAssociationLabelName).
		DoGarbageCollection()
	if err != nil {
		log.Error(err, "user garbage collector failed")
		os.Exit(1)
	}
}

func setupWebhook(mgr manager.Manager, certRotation certificates.RotationParams, clientset kubernetes.Interface) {
	manageWebhookCerts := viper.GetBool(operator.ManageWebhookCertsFlag)
	if manageWebhookCerts {
		log.Info("Automatic management of the webhook certificates enabled")
		// Ensure that all the certificates needed by the webhook server are already created
		webhookParams := webhook.Params{
			Name:       viper.GetString(operator.WebhookNameFlag),
			Namespace:  viper.GetString(operator.OperatorNamespaceFlag),
			SecretName: viper.GetString(operator.WebhookSecretFlag),
			Rotation:   certRotation,
		}

		// Force a first reconciliation to create the resources before the server is started
		if err := webhookParams.ReconcileResources(context.Background(), clientset); err != nil {
			log.Error(err, "unable to setup and fill the webhook certificates")
			os.Exit(1)
		}

		if err := webhook.Add(mgr, webhookParams, clientset); err != nil {
			log.Error(err, "unable to create controller", "controller", webhook.ControllerName)
			os.Exit(1)
		}
	}

	// setup webhooks for supported types
	webhookObjects := []interface {
		runtime.Object
		SetupWebhookWithManager(manager.Manager) error
	}{
		&apmv1.ApmServer{},
		&apmv1beta1.ApmServer{},
		&beatv1beta1.Beat{},
		&entv1beta1.EnterpriseSearch{},
		&esv1.Elasticsearch{},
		&esv1beta1.Elasticsearch{},
		&kbv1.Kibana{},
		&kbv1beta1.Kibana{},
	}
	for _, obj := range webhookObjects {
		if err := obj.SetupWebhookWithManager(mgr); err != nil {
			gvk := obj.GetObjectKind().GroupVersionKind()
			log.Error(err, "Failed to setup webhook", "group", gvk.Group, "version", gvk.Version, "kind", gvk.Kind)
		}
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
		log.Error(err, "Timeout elapsed waiting for webhook certificate to be available", "path", keyPath, "timeout_seconds", timeout.Seconds())
		os.Exit(1)
	}
}
