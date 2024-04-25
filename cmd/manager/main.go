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

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.elastic.co/apm/v2"
	"go.uber.org/automaxprocs/maxprocs"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	crwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/elastic/cloud-on-k8s/v2/pkg/about"
	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	apmv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1beta1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	esv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1beta1"
	entv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1"
	entv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1beta1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	kbv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1beta1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	emsv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/maps/v1alpha1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/agent"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/apmserver"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	associationctl "github.com/elastic/cloud-on-k8s/v2/pkg/controller/association/controller"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/autoscaling"
	esavalidation "github.com/elastic/cloud-on-k8s/v2/pkg/controller/autoscaling/elasticsearch/validation"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/beat"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/container"
	commonlicense "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	controllerscheme "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing/apmclientgo"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	commonwebhook "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/webhook"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/settings"
	esvalidation "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/validation"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/enterprisesearch"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/license"
	licensetrial "github.com/elastic/cloud-on-k8s/v2/pkg/controller/license/trial"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash"
	lsvalidation "github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/validation"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/maps"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/remoteca"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/stackconfigpolicy"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/webhook"
	"github.com/elastic/cloud-on-k8s/v2/pkg/dev"
	"github.com/elastic/cloud-on-k8s/v2/pkg/dev/portforward"
	licensing "github.com/elastic/cloud-on-k8s/v2/pkg/license"
	"github.com/elastic/cloud-on-k8s/v2/pkg/telemetry"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/cryptutil"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/fs"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	logconf "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/metrics"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/net"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/rbac"
)

const (
	DefaultMetricPort  = 0 // disabled
	DefaultWebhookName = "elastic-webhook.k8s.elastic.co"
	WebhookPort        = 9443

	LeaderElectionLeaseName = "elastic-operator-leader"

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
	cmd.Flags().String(
		operator.CADirFlag,
		"",
		"Path to a directory containing a CA certificate (tls.crt) and its associated private key (tls.key) to be used for all managed resources. Effectively disables the CA rotation and validity options.",
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
		operator.ContainerRepositoryFlag,
		"",
		"Container repository to use when downloading Elastic Stack container images",
	)
	cmd.Flags().String(
		operator.ContainerSuffixFlag,
		"",
		fmt.Sprintf("Suffix to be appended to container images by default. Cannot be combined with %s", operator.UBIOnlyFlag),
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
	cmd.Flags().Duration(
		operator.ElasticsearchObservationIntervalFlag,
		10*time.Second,
		"Interval between observations of Elasticsearch health, non-positive values disable asynchronous observation",
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
	cmd.Flags().Int(
		operator.PasswordHashCacheSize,
		0,
		fmt.Sprintf(
			"Sets the size of the password hash cache. Default size is inferred from %s. Caching is disabled if explicitly set to 0 or any negative value.",
			operator.MaxConcurrentReconcilesFlag,
		),
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
	cmd.Flags().Float32(
		operator.KubeClientQPS,
		0,
		"Maximum number of queries per second to the Kubernetes API.",
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
		"Port to use for exposing metrics in the Prometheus format. (set 0 to disable)",
	)
	cmd.Flags().String(
		operator.MetricsHostFlag,
		"0.0.0.0",
		fmt.Sprintf("The host to which the operator should bind to serve metrics in the Prometheus format. Will be combined with %s.", operator.MetricsPortFlag),
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
		fmt.Sprintf("Use only UBI container images to deploy Elastic Stack applications. UBI images are only available from 7.10.0 onward. Cannot be combined with %s", operator.ContainerSuffixFlag),
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
	cmd.Flags().Int(
		operator.WebhookPortFlag,
		WebhookPort,
		"Port is the port that the webhook server serves at.",
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

	// receive config/CA file update events over a channel
	confUpdateChan := make(chan struct{}, 1)
	var toWatch []string

	// watch for config file changes
	if !viper.GetBool(operator.DisableConfigWatch) && configFile != "" {
		toWatch = append(toWatch, configFile)
	}

	// watch for CA files if configured
	caDir := viper.GetString(operator.CADirFlag)
	if caDir != "" {
		toWatch = append(toWatch,
			filepath.Join(caDir, certificates.KeyFileName),
			filepath.Join(caDir, certificates.CertFileName),
			filepath.Join(caDir, certificates.CAKeyFileName),
			filepath.Join(caDir, certificates.CAFileName),
		)
	}

	onConfChange := func(_ []string) {
		confUpdateChan <- struct{}{}
	}
	watcher := fs.NewFileWatcher(ctx, toWatch, onConfChange, 15*time.Second)
	go watcher.Run()

	// set up channels and context for the operator
	errChan := make(chan error, 1)
	ctx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()

	// start the operator in a goroutine
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
			Addr:              viper.GetString(operator.DebugHTTPListenFlag),
			Handler:           mux,
			ReadHeaderTimeout: 60 * time.Second,
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

	// set the default container repository
	containerRepository := viper.GetString(operator.ContainerRepositoryFlag)
	if containerRepository != "" {
		log.Info("Setting default container repository", "container_repository", containerRepository)
		container.SetContainerRepository(containerRepository)
	}

	// allow users to specify a container suffix unless --ubi-only mode is active
	suffix := viper.GetString(operator.ContainerSuffixFlag)
	if len(suffix) > 0 {
		if viper.IsSet(operator.UBIOnlyFlag) {
			err := fmt.Errorf("must not combine %s and %s flags", operator.UBIOnlyFlag, operator.ContainerSuffixFlag)
			log.Error(err, "Illegal flag combination")
			return err
		}
		container.SetContainerSuffix(suffix)
	}

	// enforce UBI stack images if requested
	ubiOnly := viper.GetBool(operator.UBIOnlyFlag)
	if ubiOnly {
		container.SetContainerSuffix(container.UBISuffix)
		version.GlobalMinStackVersion = version.From(7, 10, 0)
	}

	// Get a config to talk to the apiserver
	cfg, err := ctrl.GetConfig()
	if err != nil {
		log.Error(err, "Failed to obtain client configuration")
		return err
	}

	if qps := float32(viper.GetFloat64(operator.KubeClientQPS)); qps > 0 {
		cfg.QPS = qps
		cfg.Burst = int(qps * 2)
	}

	// set up APM  tracing if configured
	var tracer *apm.Tracer
	if viper.GetBool(operator.EnableTracingFlag) {
		tracer = tracing.NewTracer("elastic-operator")
		// set up APM tracing for client-go
		cfg.Wrap(tracing.ClientGoTransportWrapper(
			apmclientgo.WithDefaultTransaction(tracing.ClientGoCacheTx(tracer)),
		))
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
		LeaderElection:             viper.GetBool(operator.EnableLeaderElection),
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
		LeaderElectionID:           LeaderElectionLeaseName,
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

	default:
		log.Info("Operator configured to manage multiple namespaces", "namespaces", managedNamespaces, "operator_namespace", operatorNamespace)
		// The managed cache should always include the operator namespace so that we can work with operator-internal resources.
		managedNamespaces = append(managedNamespaces, operatorNamespace)
	}

	// implicitly allows watching cluster-scoped resources (e.g. storage classes)
	opts.Cache = cache.Options{DefaultNamespaces: map[string]cache.Config{}}
	for _, ns := range managedNamespaces {
		opts.Cache.DefaultNamespaces[ns] = cache.Config{}
	}

	// only expose prometheus metrics if provided a non-zero port
	metricsPort := viper.GetInt(operator.MetricsPortFlag)
	metricsHost := viper.GetString(operator.MetricsHostFlag)
	if metricsPort != 0 {
		log.Info("Exposing Prometheus metrics on /metrics", "bindAddress", fmt.Sprintf("%s:%d", metricsHost, metricsPort))
	}
	opts.Metrics = metricsserver.Options{
		BindAddress: fmt.Sprintf("%s:%d", metricsHost, metricsPort), // 0 to disable
	}

	webhookPort := viper.GetInt(operator.WebhookPortFlag)
	webhookCertDir := viper.GetString(operator.WebhookCertDirFlag)
	opts.WebhookServer = crwebhook.NewServer(crwebhook.Options{
		Port:    webhookPort,
		CertDir: webhookCertDir,
	})

	mgr, err := ctrl.NewManager(cfg, opts)
	if err != nil {
		log.Error(err, "Failed to create controller manager")
		return err
	}

	// Retrieve globally shared CA if any
	ca, err := readOptionalCA(viper.GetString(operator.CADirFlag))
	if err != nil {
		log.Error(err, "Cannot read global CA")
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

	// default hash cache is arbitrarily set to 5 x MaxConcurrentReconcilesFlag
	hashCacheSize := viper.GetInt(operator.MaxConcurrentReconcilesFlag) * 5
	if viper.IsSet(operator.PasswordHashCacheSize) {
		hashCacheSize = viper.GetInt(operator.PasswordHashCacheSize)
	}
	passwordHasher, err := cryptutil.NewPasswordHasher(hashCacheSize)
	if err != nil {
		log.Error(err, "failed to create hash cache")
		return err
	}

	params := operator.Parameters{
		Dialer:                           dialer,
		ElasticsearchObservationInterval: viper.GetDuration(operator.ElasticsearchObservationIntervalFlag),
		ExposedNodeLabels:                exposedNodeLabels,
		IPFamily:                         ipFamily,
		OperatorNamespace:                operatorNamespace,
		OperatorInfo:                     operatorInfo,
		GlobalCA:                         ca,
		CACertRotation: certificates.RotationParams{
			Validity:     caCertValidity,
			RotateBefore: caCertRotateBefore,
		},
		CertRotation: certificates.RotationParams{
			Validity:     certValidity,
			RotateBefore: certRotateBefore,
		},
		PasswordHasher:            passwordHasher,
		MaxConcurrentReconciles:   viper.GetInt(operator.MaxConcurrentReconcilesFlag),
		SetDefaultSecurityContext: setDefaultSecurityContext,
		ValidateStorageClass:      viper.GetBool(operator.ValidateStorageClassFlag),
		Tracer:                    tracer,
	}

	if viper.GetBool(operator.EnableWebhookFlag) {
		setupWebhook(ctx, mgr, params, webhookCertDir, clientset, exposedNodeLabels, managedNamespaces, tracer)
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
	go asyncTasks(ctx, mgr, cfg, managedNamespaces, operatorNamespace, operatorInfo, disableTelemetry, telemetryInterval, tracer)

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
		licenseType, err := lc.ValidOperatorLicenseKeyType(ctx)
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

func readOptionalCA(caDir string) (*certificates.CA, error) {
	if caDir == "" {
		return nil, nil
	}
	return certificates.BuildCAFromFile(caDir)
}

// asyncTasks schedules some tasks to be started when this instance of the operator is elected
func asyncTasks(
	ctx context.Context,
	mgr manager.Manager,
	cfg *rest.Config,
	managedNamespaces []string,
	operatorNamespace string,
	operatorInfo about.OperatorInfo,
	disableTelemetry bool,
	telemetryInterval time.Duration,
	tracer *apm.Tracer,
) {
	<-mgr.Elected() // wait for this operator instance to be elected

	// Report this instance as elected through Prometheus
	metrics.Leader.WithLabelValues(string(operatorInfo.OperatorUUID), operatorNamespace).Set(1)

	time.Sleep(10 * time.Second)         // wait some arbitrary time for the manager to start
	mgr.GetCache().WaitForCacheSync(ctx) // wait until k8s client cache is initialized

	// Start the resource reporter
	go func() {
		r := licensing.NewResourceReporter(mgr.GetClient(), operatorNamespace, tracer)
		r.Start(ctx, licensing.ResourceReporterFrequency)
	}()

	if !disableTelemetry {
		// Start the telemetry reporter
		go func() {
			tr := telemetry.NewReporter(operatorInfo, mgr.GetClient(), operatorNamespace, managedNamespaces, telemetryInterval, tracer)
			tr.Start(ctx)
		}()
	}

	// Garbage collect orphaned secrets leftover from deleted resources while the operator was not running
	// - association user secrets
	gcCtx := tracing.NewContextTransaction(ctx, tracer, tracing.RunOnceTxType, "garbage-collection", nil)
	err := garbageCollectUsers(gcCtx, cfg, managedNamespaces)
	if err != nil {
		log.Error(err, "exiting due to unrecoverable error")
		os.Exit(1)
	}
	// - soft-owned secrets
	garbageCollectSoftOwnedSecrets(gcCtx, mgr.GetClient())
	tracing.EndContextTransaction(gcCtx)
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
//  1. If the setDefaultSecurityContext is explicitly set to either true, or false, use this value.
//  2. use OpenShift detection to determine whether or not we are running within an OpenShift cluster.
//     If we determine we are on an OpenShift cluster, and since OpenShift automatically sets security context, return false,
//     otherwise, return true as we'll need to set this security context on non-OpenShift clusters.
func determineSetDefaultSecurityContext(setDefaultSecurityContext string, clientset kubernetes.Interface) (bool, error) {
	if setDefaultSecurityContext == "auto-detect" {
		openshift, err := isOpenShift(clientset)
		return !openshift, err
	}
	return strconv.ParseBool(setDefaultSecurityContext)
}

// isOpenShift detects whether we are running on OpenShift. Detection inspired by kubevirt:
// - https://github.com/kubevirt/kubevirt/blob/f71e9c9615a6c36178169d66814586a93ba515b5/pkg/util/cluster/cluster.go#L21
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
		{name: "StackConfigPolicy", registerFunc: stackconfigpolicy.Add},
		{name: "Logstash", registerFunc: logstash.Add},
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
		{name: "LOGSTASH-ES", registerFunc: associationctl.AddLogstashES},
		{name: "ES-MONITORING", registerFunc: associationctl.AddEsMonitoring},
		{name: "KB-MONITORING", registerFunc: associationctl.AddKbMonitoring},
		{name: "BEAT-MONITORING", registerFunc: associationctl.AddBeatMonitoring},
		{name: "LOGSTASH-MONITORING", registerFunc: associationctl.AddLogstashMonitoring},
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

func garbageCollectUsers(ctx context.Context, cfg *rest.Config, managedNamespaces []string) error {
	span, ctx := apm.StartSpan(ctx, "gc_users", tracing.SpanTypeApp)
	defer span.End()

	ugc, err := association.NewUsersGarbageCollector(cfg, managedNamespaces)
	if err != nil {
		return fmt.Errorf("user garbage collector creation failed: %w", err)
	}
	err = ugc.
		For(&apmv1.ApmServerList{}, associationctl.ApmAssociationLabelNamespace, associationctl.ApmAssociationLabelName).
		For(&kbv1.KibanaList{}, associationctl.KibanaAssociationLabelNamespace, associationctl.KibanaAssociationLabelName).
		For(&entv1.EnterpriseSearchList{}, associationctl.EntESAssociationLabelNamespace, associationctl.EntESAssociationLabelName).
		For(&beatv1beta1.BeatList{}, associationctl.BeatAssociationLabelNamespace, associationctl.BeatAssociationLabelName).
		For(&agentv1alpha1.AgentList{}, associationctl.AgentAssociationLabelNamespace, associationctl.AgentAssociationLabelName).
		For(&emsv1alpha1.ElasticMapsServerList{}, associationctl.MapsESAssociationLabelNamespace, associationctl.MapsESAssociationLabelName).
		For(&logstashv1alpha1.LogstashList{}, associationctl.LogstashAssociationLabelNamespace, associationctl.LogstashAssociationLabelName).
		DoGarbageCollection(ctx)
	if err != nil {
		return fmt.Errorf("user garbage collector failed: %w", err)
	}
	return nil
}

func garbageCollectSoftOwnedSecrets(ctx context.Context, k8sClient k8s.Client) {
	span, ctx := apm.StartSpan(ctx, "gc_soft_owned_secrets", tracing.SpanTypeApp)
	defer span.End()

	if err := reconciler.GarbageCollectAllSoftOwnedOrphanSecrets(ctx, k8sClient, map[string]client.Object{
		esv1.Kind:             &esv1.Elasticsearch{},
		apmv1.Kind:            &apmv1.ApmServer{},
		kbv1.Kind:             &kbv1.Kibana{},
		entv1.Kind:            &entv1.EnterpriseSearch{},
		beatv1beta1.Kind:      &beatv1beta1.Beat{},
		agentv1alpha1.Kind:    &agentv1alpha1.Agent{},
		emsv1alpha1.Kind:      &emsv1alpha1.ElasticMapsServer{},
		policyv1alpha1.Kind:   &policyv1alpha1.StackConfigPolicy{},
		logstashv1alpha1.Kind: &logstashv1alpha1.Logstash{},
	}); err != nil {
		log.Error(err, "Orphan secrets garbage collection failed, will be attempted again at next operator restart.")
		return
	}
	log.Info("Orphan secrets garbage collection complete")
}

func setupWebhook(
	ctx context.Context,
	mgr manager.Manager,
	params operator.Parameters,
	webhookCertDir string,
	clientset kubernetes.Interface,
	exposedNodeLabels esvalidation.NodeLabels,
	managedNamespaces []string,
	tracer *apm.Tracer) {
	manageWebhookCerts := viper.GetBool(operator.ManageWebhookCertsFlag)
	if manageWebhookCerts {
		if err := reconcileWebhookCertsAndAddController(ctx, mgr, params.CertRotation, clientset, tracer); err != nil {
			log.Error(err, "unable to setup the webhook certificates")
			os.Exit(1)
		}
	}

	checker := commonlicense.NewLicenseChecker(mgr.GetClient(), params.OperatorNamespace)
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
		&policyv1alpha1.StackConfigPolicy{},
	}
	for _, obj := range webhookObjects {
		if err := commonwebhook.SetupValidatingWebhookWithConfig(&commonwebhook.Config{
			Manager:          mgr,
			WebhookPath:      obj.WebhookPath(),
			ManagedNamespace: managedNamespaces,
			Validator:        obj,
			LicenseChecker:   checker,
		}); err != nil {
			gvk := obj.GetObjectKind().GroupVersionKind()
			log.Error(err, "Failed to setup webhook", "group", gvk.Group, "version", gvk.Version, "kind", gvk.Kind)
		}
	}

	// Logstash, Elasticsearch and ElasticsearchAutoscaling validating webhooks are wired up differently, in order to access the k8s client
	esvalidation.RegisterWebhook(mgr, params.ValidateStorageClass, exposedNodeLabels, checker, managedNamespaces)
	esavalidation.RegisterWebhook(mgr, params.ValidateStorageClass, checker, managedNamespaces)
	lsvalidation.RegisterWebhook(mgr, params.ValidateStorageClass, managedNamespaces)

	// wait for the secret to be populated in the local filesystem before returning
	interval := time.Second * 1
	timeout := time.Second * 30
	keyPath := filepath.Join(webhookCertDir, certificates.CertFileName)
	log.Info("Polling for the webhook certificate to be available", "path", keyPath)
	//nolint:staticcheck
	err := wait.PollImmediateWithContext(ctx, interval, timeout, func(_ context.Context) (bool, error) {
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

func reconcileWebhookCertsAndAddController(ctx context.Context, mgr manager.Manager, certRotation certificates.RotationParams, clientset kubernetes.Interface, tracer *apm.Tracer) error {
	ctx = tracing.NewContextTransaction(ctx, tracer, tracing.ReconciliationTxType, webhook.ControllerName, nil)
	defer tracing.EndContextTransaction(ctx)
	log.Info("Automatic management of the webhook certificates enabled")
	// Ensure that all the certificates needed by the webhook server are already created
	webhookParams := webhook.Params{
		Name:       viper.GetString(operator.WebhookNameFlag),
		Namespace:  viper.GetString(operator.OperatorNamespaceFlag),
		SecretName: viper.GetString(operator.WebhookSecretFlag),
		Rotation:   certRotation,
	}

	// retrieve the current webhook configuration interface
	wh, err := webhookParams.NewAdmissionControllerInterface(ctx, clientset)
	if err != nil {
		return err
	}

	// Force a first reconciliation to create the resources before the server is started
	if err := webhookParams.ReconcileResources(ctx, clientset, wh); err != nil {
		return err
	}

	return webhook.Add(mgr, webhookParams, clientset, wh, tracer)
}
