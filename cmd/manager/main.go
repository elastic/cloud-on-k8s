// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package manager

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.elastic.co/apm"
	"go.uber.org/automaxprocs/maxprocs"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // allow gcp authentication
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	apmv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1beta1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	esv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	entv1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1"
	entv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	kbv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1beta1"
	emsv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/maps/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/agent"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	associationctl "github.com/elastic/cloud-on-k8s/pkg/controller/association/controller"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	commonlicense "github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	controllerscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	commonwebhook "github.com/elastic/cloud-on-k8s/pkg/controller/common/webhook"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	esvalidation "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/validation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/enterprisesearch"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/pkg/controller/license"
	licensetrial "github.com/elastic/cloud-on-k8s/pkg/controller/license/trial"
	"github.com/elastic/cloud-on-k8s/pkg/controller/maps"
	"github.com/elastic/cloud-on-k8s/pkg/controller/remoteca"
	"github.com/elastic/cloud-on-k8s/pkg/controller/webhook"
	"github.com/elastic/cloud-on-k8s/pkg/dev"
	"github.com/elastic/cloud-on-k8s/pkg/dev/portforward"
	licensing "github.com/elastic/cloud-on-k8s/pkg/license"
	"github.com/elastic/cloud-on-k8s/pkg/telemetry"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	logconf "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/pkg/utils/metrics"
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
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
		operator.DebugHTTPListenFlag,
		"localhost:6060",
		"Listen address for debug HTTP server (only available in development mode)",
	)
	cmd.Flags().Bool(
		operator.DisableConfigWatch,
		false,
		"Disable watching the configuration file for changes",
	)
	cmd.Flags().Duration(
		operator.ElasticsearchClientTimeout,
		3*time.Minute,
		"Default timeout for requests made by the Elasticsearch client.",
	)
	cmd.Flags().Bool(
		operator.DisableTelemetryFlag,
		false,
		"Disable periodically updating ECK telemetry data for Kibana to consume.",
	)
	cmd.Flags().String(
		operator.DistributionChannelFlag,
		"",
		"Set the distribution channel to report through telemetry.",
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
	cmd.Flags().StringSlice(
		operator.ExposedNodeLabels,
		[]string{},
		"Comma separated list of node labels which are allowed to be copied as annotations on Elasticsearch Pods, empty by default",
	)
	cmd.Flags().String(
		operator.IPFamilyFlag,
		"",
		"Set the IP family to use. Possible values: IPv4, IPv6, \"\" (= auto-detect) ",
	)
	cmd.Flags().Duration(
		operator.KubeClientTimeout,
		60*time.Second,
		"Timeout for requests made by the Kubernetes API client.",
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
	cmd.Flags().Duration(
		operator.TelemetryIntervalFlag,
		1*time.Hour,
		"Interval between ECK telemetry data updates",
	)
	cmd.Flags().Bool(
		operator.UBIOnlyFlag,
		false,
		"Use only UBI container images to deploy Elastic Stack applications. UBI images are only available from 7.10.0 onward.",
	)
	cmd.Flags().Bool(
		operator.ValidateStorageClassFlag,
		true,
		"Specifies whether the operator should retrieve storage classes to verify volume expansion support. Can be disabled if cluster-wide storage class RBAC access is not available.",
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
	cmd.Flags().String(
		operator.SetDefaultSecurityContextFlag,
		"auto-detect",
		"Enables setting the default security context with fsGroup=1000 for Elasticsearch 8.0+ Pods. Ignored pre-8.0. Possible values: true, false, auto-detect",
	)

	// hide development mode flags from the usage message
	_ = cmd.Flags().MarkHidden(operator.AutoPortForwardFlag)
	_ = cmd.Flags().MarkHidden(operator.DebugHTTPListenFlag)

	// hide flags set by the build process
	_ = cmd.Flags().MarkHidden(operator.DistributionChannelFlag)

	// hide the flag used for E2E test only
	_ = cmd.Flags().MarkHidden(operator.TelemetryIntervalFlag)

	// configure filename auto-completion for the config flag
	_ = cmd.MarkFlagFilename(operator.ConfigFlag)

	logconf.BindFlags(cmd.Flags())

	return cmd
}

func doRun(_ *cobra.Command, _ []string) error {
	ctx := signals.SetupSignalHandler()
	disableConfigWatch := viper.GetBool(operator.DisableConfigWatch)

	// no config file to watch so start the operator directly
	if configFile == "" || disableConfigWatch {
		return startOperator(ctx)
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
	ctx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()

	go func() {
		err := startOperator(ctx)
		if err != nil {
			log.Error(err, "Operator stopped with error")
		}
		errChan <- err
	}()

	// watch for events
	for {
		select {
		case err := <-errChan: // operator failed
			log.Error(err, "Shutting down due to error")

			return err
		case <-ctx.Done(): // signal received
			log.Info("Shutting down due to signal")

			return nil
		case <-confUpdateChan: // config file updated
			log.Info("Shutting down to apply updated configuration")

			return nil
		}
	}
}

func startOperator(ctx context.Context) error {
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
				<-ctx.Done()

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

	// enforce UBI stack images if requested
	ubiOnly := viper.GetBool(operator.UBIOnlyFlag)
	if ubiOnly {
		container.SetContainerSuffix("-ubi8")
		version.GlobalMinStackVersion = version.From(7, 10, 0)
	}

	// Get a config to talk to the apiserver
	cfg, err := ctrl.GetConfig()
	if err != nil {
		log.Error(err, "Failed to obtain client configuration")
		return err
	}

	// set the timeout for API client
	cfg.Timeout = viper.GetDuration(operator.KubeClientTimeout)

	// set the timeout for Elasticsearch requests
	esclient.DefaultESClientTimeout = viper.GetDuration(operator.ElasticsearchClientTimeout)

	// Setup Scheme for all resources
	log.Info("Setting up scheme")
	controllerscheme.SetupScheme()
	// also set up the v1beta1 scheme, used by the v1beta1 webhook
	controllerscheme.SetupV1beta1Scheme()

	// Create a new Cmd to provide shared dependencies and start components
	opts := ctrl.Options{
		Scheme:                     clientgoscheme.Scheme,
		CertDir:                    viper.GetString(operator.WebhookCertDirFlag),
		LeaderElection:             viper.GetBool(operator.EnableLeaderElection),
		LeaderElectionResourceLock: resourcelock.ConfigMapsResourceLock, // TODO: Revert to ConfigMapsLeases when support for 1.13 is dropped
		LeaderElectionID:           LeaderElectionConfigMapName,
		LeaderElectionNamespace:    operatorNamespace,
		Logger:                     log.WithName("eck-operator"),
	}

	// configure the manager cache based on the number of managed namespaces
	managedNamespaces := viper.GetStringSlice(operator.NamespacesFlag)
	switch {
	case len(managedNamespaces) == 0:
		log.Info("Operator configured to manage all namespaces")
	case len(managedNamespaces) == 1 && managedNamespaces[0] == operatorNamespace:
		log.Info("Operator configured to manage a single namespace", "namespace", managedNamespaces[0], "operator_namespace", operatorNamespace)
		// opts.Namespace implicitly allows watching cluster-scoped resources (e.g. storage classes)
		opts.Namespace = managedNamespaces[0]
	default:
		log.Info("Operator configured to manage multiple namespaces", "namespaces", managedNamespaces, "operator_namespace", operatorNamespace)
		// The managed cache should always include the operator namespace so that we can work with operator-internal resources.
		managedNamespaces = append(managedNamespaces, operatorNamespace)

		opts.NewCache = cache.MultiNamespacedCacheBuilder(managedNamespaces)
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

	ipFamily, err := chooseAndValidateIPFamily(viper.GetString(operator.IPFamilyFlag), net.ToIPFamily(os.Getenv(settings.EnvPodIP)))
	if err != nil {
		log.Error(err, "Invalid IP family parameter")
		return err
	}

	// Setup a client to set the operator uuid config map
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "Failed to create Kubernetes client")
		return err
	}

	distributionChannel := viper.GetString(operator.DistributionChannelFlag)
	operatorInfo, err := about.GetOperatorInfo(clientset, operatorNamespace, distributionChannel)
	if err != nil {
		log.Error(err, "Failed to get operator info")
		return err
	}

	log.Info("Setting up controllers")
	var tracer *apm.Tracer
	if viper.GetBool(operator.EnableTracingFlag) {
		tracer = tracing.NewTracer("elastic-operator")
	}

	exposedNodeLabels, err := esvalidation.NewExposedNodeLabels(viper.GetStringSlice(operator.ExposedNodeLabels))
	if err != nil {
		log.Error(err, "Failed to parse exposed node labels")
		return err
	}

	setDefaultSecurityContext, err := determineSetDefaultSecurityContext(viper.GetString(operator.SetDefaultSecurityContextFlag), clientset)
	if err != nil {
		log.Error(err, "failed to determine how to set default security context")
		return err
	}

	params := operator.Parameters{
		Dialer:            dialer,
		ExposedNodeLabels: exposedNodeLabels,
		IPFamily:          ipFamily,
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
		SetDefaultSecurityContext: setDefaultSecurityContext,
		ValidateStorageClass:      viper.GetBool(operator.ValidateStorageClassFlag),
		Tracer:                    tracer,
	}

	if viper.GetBool(operator.EnableWebhookFlag) {
		setupWebhook(mgr, params.CertRotation, params.ValidateStorageClass, clientset, exposedNodeLabels, managedNamespaces)
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

	disableTelemetry := viper.GetBool(operator.DisableTelemetryFlag)
	telemetryInterval := viper.GetDuration(operator.TelemetryIntervalFlag)
	go asyncTasks(mgr, cfg, managedNamespaces, operatorNamespace, operatorInfo, disableTelemetry, telemetryInterval)

	log.Info("Starting the manager", "uuid", operatorInfo.OperatorUUID,
		"namespace", operatorNamespace, "version", operatorInfo.BuildInfo.Version,
		"build_hash", operatorInfo.BuildInfo.Hash, "build_date", operatorInfo.BuildInfo.Date,
		"build_snapshot", operatorInfo.BuildInfo.Snapshot)

	exitOnErr := make(chan error)

	// start the manager
	go func() {
		if err := mgr.Start(ctx); err != nil {
			log.Error(err, "Failed to start the controller manager")
			exitOnErr <- err
		}
	}()

	// check operator license key
	go func() {
		mgr.GetCache().WaitForCacheSync(ctx)

		lc := commonlicense.NewLicenseChecker(mgr.GetClient(), params.OperatorNamespace)
		licenseType, err := lc.ValidOperatorLicenseKeyType()
		if err != nil {
			log.Error(err, "Failed to validate operator license key")
			exitOnErr <- err
		} else {
			log.Info("Operator license key validated", "license_type", licenseType)
		}
	}()

	for {
		select {
		case err = <-exitOnErr:
			return err
		case <-ctx.Done():
			return nil
		}
	}
}

// asyncTasks schedules some tasks to be started when this instance of the operator is elected
func asyncTasks(
	mgr manager.Manager,
	cfg *rest.Config,
	managedNamespaces []string,
	operatorNamespace string,
	operatorInfo about.OperatorInfo,
	disableTelemetry bool,
	telemetryInterval time.Duration,
) {
	<-mgr.Elected() // wait for this operator instance to be elected

	// Report this instance as elected through Prometheus
	metrics.Leader.WithLabelValues(string(operatorInfo.OperatorUUID), operatorNamespace).Set(1)

	time.Sleep(10 * time.Second)                          // wait some arbitrary time for the manager to start
	mgr.GetCache().WaitForCacheSync(context.Background()) // wait until k8s client cache is initialized

	// Start the resource reporter
	go func() {
		r := licensing.NewResourceReporter(mgr.GetClient(), operatorNamespace)
		r.Start(licensing.ResourceReporterFrequency)
	}()

	if !disableTelemetry {
		// Start the telemetry reporter
		go func() {
			tr := telemetry.NewReporter(operatorInfo, mgr.GetClient(), operatorNamespace, managedNamespaces, telemetryInterval)
			tr.Start()
		}()
	}

	// Garbage collect orphaned secrets leftover from deleted resources while the operator was not running
	// - association user secrets
	garbageCollectUsers(cfg, managedNamespaces)
	// - soft-owned secrets
	garbageCollectSoftOwnedSecrets(mgr.GetClient())
}

func chooseAndValidateIPFamily(ipFamilyStr string, ipFamilyDefault corev1.IPFamily) (corev1.IPFamily, error) {
	switch strings.ToLower(ipFamilyStr) {
	case "":
		return ipFamilyDefault, nil
	case "ipv4":
		return corev1.IPv4Protocol, nil
	case "ipv6":
		return corev1.IPv6Protocol, nil
	default:
		return ipFamilyDefault, fmt.Errorf("IP family can be one of: IPv4, IPv6 or \"\" to auto-detect, but was %s", ipFamilyStr)
	}
}

// determineSetDefaultSecurityContext determines what settings we need to use for security context by using the following rules:
// 1. If the setDefaultSecurityContext is explicitly set to either true, or false, use this value.
// 2. use OpenShift detection to determine whether or not we are running within an OpenShift cluster.
//    If we determine we are on an OpenShift cluster, and since OpenShift automatically sets security context, return false,
//    otherwise, return true as we'll need to set this security context on non-OpenShift clusters.
func determineSetDefaultSecurityContext(setDefaultSecurityContext string, clientset kubernetes.Interface) (bool, error) {
	if setDefaultSecurityContext == "auto-detect" {
		openshift, err := isOpenShift(clientset)
		return !openshift, err
	}
	return strconv.ParseBool(setDefaultSecurityContext)
}

// isOpenShift detects whether we are running on OpenShift.  Detection inspired by kubevirt
//    https://github.com/kubevirt/kubevirt/blob/f71e9c9615a6c36178169d66814586a93ba515b5/pkg/util/cluster/cluster.go#L21
func isOpenShift(clientset kubernetes.Interface) (bool, error) {
	openshiftSecurityGroupVersion := schema.GroupVersion{Group: "security.openshift.io", Version: "v1"}
	apiResourceList, err := clientset.Discovery().ServerResourcesForGroupVersion(openshiftSecurityGroupVersion.String())
	if err != nil {
		// In case of an error, check if security.openshift.io is the reason (unlikely).
		var e *discovery.ErrGroupDiscoveryFailed
		if ok := errors.As(err, &e); ok {
			if _, exists := e.Groups[openshiftSecurityGroupVersion]; exists {
				// If security.openshift.io is the reason for the error, we are absolutely on OpenShift
				return true, nil
			}
		}
		// If the security.openshift.io group isn't found, we are not on OpenShift
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	// Search for "securitycontextconstraints" within the cluster's API resources,
	// since this is an OpenShift specific API resource that does not exist outside of OpenShift.
	for _, apiResource := range apiResourceList.APIResources {
		if apiResource.Name == "securitycontextconstraints" {
			// we have determined we are absolutely running on OpenShift
			return true, nil
		}
	}

	// We could not determine that we are running on an OpenShift cluster,
	// so we will behave as if "setDefaultSecurityContext" was set to true.
	return false, nil
}

func registerControllers(mgr manager.Manager, params operator.Parameters, accessReviewer rbac.AccessReviewer) error {
	controllers := []struct {
		name         string
		registerFunc func(manager.Manager, operator.Parameters) error
	}{
		{name: "APMServer", registerFunc: apmserver.Add},
		{name: "Elasticsearch", registerFunc: elasticsearch.Add},
		{name: "ElasticsearchAutoscaling", registerFunc: autoscaling.Add},
		{name: "Kibana", registerFunc: kibana.Add},
		{name: "EnterpriseSearch", registerFunc: enterprisesearch.Add},
		{name: "Beats", registerFunc: beat.Add},
		{name: "License", registerFunc: license.Add},
		{name: "LicenseTrial", registerFunc: licensetrial.Add},
		{name: "Agent", registerFunc: agent.Add},
		{name: "Maps", registerFunc: maps.Add},
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
		{name: "KB-ENT", registerFunc: associationctl.AddKibanaEnt},
		{name: "ENT-ES", registerFunc: associationctl.AddEntES},
		{name: "BEAT-ES", registerFunc: associationctl.AddBeatES},
		{name: "BEAT-KB", registerFunc: associationctl.AddBeatKibana},
		{name: "AGENT-ES", registerFunc: associationctl.AddAgentES},
		{name: "AGENT-KB", registerFunc: associationctl.AddAgentKibana},
		{name: "AGENT-FS", registerFunc: associationctl.AddAgentFleetServer},
		{name: "EMS-ES", registerFunc: associationctl.AddMapsES},
		{name: "ES-MONITORING", registerFunc: associationctl.AddEsMonitoring},
		{name: "KB-MONITORING", registerFunc: associationctl.AddKbMonitoring},
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
		For(&kbv1.KibanaList{}, associationctl.KibanaAssociationLabelNamespace, associationctl.KibanaAssociationLabelName).
		For(&entv1.EnterpriseSearchList{}, associationctl.EntESAssociationLabelNamespace, associationctl.EntESAssociationLabelName).
		For(&beatv1beta1.BeatList{}, associationctl.BeatAssociationLabelNamespace, associationctl.BeatAssociationLabelName).
		For(&agentv1alpha1.AgentList{}, associationctl.AgentAssociationLabelNamespace, associationctl.AgentAssociationLabelName).
		For(&emsv1alpha1.ElasticMapsServerList{}, associationctl.MapsESAssociationLabelNamespace, associationctl.MapsESAssociationLabelName).
		DoGarbageCollection()
	if err != nil {
		log.Error(err, "user garbage collector failed")
		os.Exit(1)
	}
}

func garbageCollectSoftOwnedSecrets(k8sClient k8s.Client) {
	if err := reconciler.GarbageCollectAllSoftOwnedOrphanSecrets(k8sClient, map[string]client.Object{
		esv1.Kind:          &esv1.Elasticsearch{},
		apmv1.Kind:         &apmv1.ApmServer{},
		kbv1.Kind:          &kbv1.Kibana{},
		entv1.Kind:         &entv1.EnterpriseSearch{},
		beatv1beta1.Kind:   &beatv1beta1.Beat{},
		agentv1alpha1.Kind: &agentv1alpha1.Agent{},
		emsv1alpha1.Kind:   &emsv1alpha1.ElasticMapsServer{},
	}); err != nil {
		log.Error(err, "Orphan secrets garbage collection failed, will be attempted again at next operator restart.")
		return
	}
	log.Info("Orphan secrets garbage collection complete")
}

func setupWebhook(
	mgr manager.Manager,
	certRotation certificates.RotationParams,
	validateStorageClass bool,
	clientset kubernetes.Interface,
	exposedNodeLabels esvalidation.NodeLabels,
	managedNamespaces []string) {
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

		// retrieve the current webhook configuration interface
		wh, err := webhookParams.NewAdmissionControllerInterface(context.Background(), clientset)
		if err != nil {
			log.Error(err, "unable to setup the webhook certificates")
			os.Exit(1)
		}

		// Force a first reconciliation to create the resources before the server is started
		if err := webhookParams.ReconcileResources(context.Background(), clientset, wh); err != nil {
			log.Error(err, "unable to setup the webhook certificates")
			os.Exit(1)
		}

		if err := webhook.Add(mgr, webhookParams, clientset, wh); err != nil {
			log.Error(err, "unable to create controller", "controller", webhook.ControllerName)
			os.Exit(1)
		}
	}

	// setup webhooks for supported types
	webhookObjects := []interface {
		runtime.Object
		admission.Validator
		WebhookPath() string
	}{
		&agentv1alpha1.Agent{},
		&apmv1.ApmServer{},
		&apmv1beta1.ApmServer{},
		&beatv1beta1.Beat{},
		&entv1.EnterpriseSearch{},
		&entv1beta1.EnterpriseSearch{},
		&esv1beta1.Elasticsearch{},
		&kbv1.Kibana{},
		&kbv1beta1.Kibana{},
		&emsv1alpha1.ElasticMapsServer{},
	}
	for _, obj := range webhookObjects {
		if err := commonwebhook.SetupValidatingWebhookWithConfig(&commonwebhook.Config{
			Manager:          mgr,
			WebhookPath:      obj.WebhookPath(),
			ManagedNamespace: managedNamespaces,
			Validator:        obj,
		}); err != nil {
			gvk := obj.GetObjectKind().GroupVersionKind()
			log.Error(err, "Failed to setup webhook", "group", gvk.Group, "version", gvk.Version, "kind", gvk.Kind)
		}
	}

	// esv1 validating webhook is wired up differently, in order to access the k8s client
	esvalidation.RegisterWebhook(mgr, validateStorageClass, exposedNodeLabels, managedNamespaces)

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
