// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package run

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	sprig "github.com/Masterminds/sprig/v3"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/retry"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/command"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	jobTimeout           = 600 * time.Minute // time to wait for the test job to finish
	kubePollInterval     = 10 * time.Second  // Kube API polling interval
	testRunLabel         = "test-run"        // name of the label applied to resources
	logStreamLabel       = "stream-logs"     // name of the label enabling log streaming to e2e runner
	testsLogFile         = "e2e-tests.json"  // name of file to keep all test logs in JSON format
	operatorReadyTimeout = 3 * time.Minute   // time to wait for the operator pod to be ready

	TestNameLabel = "test-name" // name of the label applied to resources during each test
)

type stepFunc func() error

func doRun(flags runFlags) error {
	helper := &helper{
		runFlags:       flags,
		kubectlWrapper: command.NewKubectl(flags.kubeConfig),
	}

	var steps []stepFunc
	if flags.local {
		// local test run steps
		steps = []stepFunc{
			helper.createScratchDir,
			helper.initTestContext,
			helper.installCRDs,
			helper.createRoles,
			helper.createManagedNamespaces,
			helper.deploySecurityConstraints,
		}
	} else {
		// CI test run steps
		steps = []stepFunc{
			helper.createScratchDir,
			helper.initTestContext,
			helper.initTestSecrets,
			helper.createE2ENamespaceAndRoleBindings,
			helper.createRoles,
			helper.installCRDs,
			helper.createOperatorNamespaces,
			helper.createManagedNamespaces,
			helper.deployTestSecrets,
			helper.deploySecurityConstraints,
			helper.deployMonitoring,
			helper.deployOperator,
			helper.waitForOperatorToBeReady,
			helper.runTestJob,
		}
	}

	defer helper.runCleanup()

	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}

	return nil
}

type helper struct {
	runFlags
	eventLog        string
	kubectlWrapper  *command.Kubectl
	testContext     test.Context
	testSecrets     map[string]string
	operatorSecrets map[string]string
	scratchDir      string
	cleanupFuncs    []func()
}

func (h *helper) createScratchDir() error {
	h.scratchDir = filepath.Join(h.scratchDirRoot, h.testRunName)
	log.Info("Creating test output directory", "directory", h.scratchDir)

	// ensure that the directory does not exist
	if _, err := os.Stat(h.scratchDir); !os.IsNotExist(err) {
		return errors.Wrapf(err, "scratch directory already exists: %s", h.scratchDir)
	}

	// create the directory
	if err := os.MkdirAll(h.scratchDir, os.ModePerm); err != nil {
		return errors.Wrapf(err, "failed to create scratch directory: %s", h.scratchDir)
	}

	// generate the path to the event log
	h.eventLog = filepath.Join(h.scratchDir, "event.log")

	// clean up the directory
	h.addCleanupFunc(func() {
		log.Info("Cleaning up the scratch directory", "directory", h.scratchDir)
		if err := os.RemoveAll(h.scratchDir); err != nil {
			log.Error(err, "Failed to cleanup scratch directory", "path", h.scratchDir)
		}
	})

	return nil
}

func (h *helper) initTestContext() error {
	h.testContext = test.Context{
		AutoPortForwarding:  h.autoPortForwarding,
		E2EImage:            h.e2eImage,
		E2ENamespace:        h.testRunName,
		E2EServiceAccount:   h.testRunName,
		ElasticStackVersion: h.elasticStackVersion,
		Local:               h.local,
		LogVerbosity:        h.logVerbosity,
		Operator: test.NamespaceOperator{
			ClusterResource: test.ClusterResource{
				Name:      fmt.Sprintf("%s-operator", h.testRunName),
				Namespace: fmt.Sprintf("%s-elastic-system", h.testRunName),
			},
			ManagedNamespaces: make([]string, len(h.managedNamespaces)),
			Replicas:          h.operatorReplicas,
		},
		OperatorImage:         h.operatorImage,
		TestLicense:           h.testLicense,
		TestLicensePKeyPath:   h.testLicensePKeyPath,
		MonitoringSecrets:     h.monitoringSecrets,
		TestRegex:             h.testRegex,
		TestRun:               h.testRunName,
		TestTimeout:           h.testTimeout,
		Pipeline:              h.pipeline,
		BuildNumber:           h.buildNumber,
		Provider:              h.provider,
		ClusterName:           h.clusterName,
		KubernetesVersion:     getKubernetesVersion(h),
		IgnoreWebhookFailures: h.ignoreWebhookFailures,
		OcpCluster:            isOcpCluster(h),
		Ocp3Cluster:           isOcp3Cluster(h),
		DeployChaosJob:        h.deployChaosJob,
		TestEnvTags:           h.testEnvTags,
	}

	for i, ns := range h.managedNamespaces {
		h.testContext.Operator.ManagedNamespaces[i] = fmt.Sprintf("%s-%s", h.testRunName, ns)
	}

	// write the test context if required
	if h.testContextOutPath != "" {
		log.Info("Writing test context", "path", h.testContextOutPath)
		f, err := os.Create(h.testContextOutPath)
		if err != nil {
			return errors.Wrap(err, "failed to write test context")
		}

		defer f.Close()
		enc := json.NewEncoder(f)
		if err := enc.Encode(h.testContext); err != nil {
			return errors.Wrap(err, "failed to encode test context")
		}
	}

	return nil
}

func getKubernetesVersion(h *helper) string {
	out, err := h.kubectl("version", "--output=json")
	if err != nil {
		panic(fmt.Sprintf("can't determine kubernetes version, err %s", err))
	}

	kubectlVersionResponse := struct {
		ServerVersion map[string]string `json:"serverVersion"`
	}{}

	if err := json.Unmarshal([]byte(out), &kubectlVersionResponse); err != nil {
		panic(fmt.Sprintf("can't determine Kubernetes version, err %s", err))
	}

	serverVersion, ok := kubectlVersionResponse.ServerVersion["gitVersion"]
	if !ok {
		panic("can't determine Kubernetes version, gitVersion missing from kubectl response")
	}

	serverVersion = strings.TrimPrefix(serverVersion, "v")
	v := version.MustParse(serverVersion)

	// we just want major and minor to aggregate test results
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

func isOcpCluster(h *helper) bool {
	_, err := h.kubectl("get", "clusterversion")
	isOCP4 := err == nil
	isOCP3 := isOcp3Cluster(h)
	return isOCP4 || isOCP3
}

func isOcp3Cluster(h *helper) bool {
	_, err := h.kubectl("get", "-n", "openshift-template-service-broker", "svc", "apiserver")
	return err == nil
}

func (h *helper) initTestSecrets() error {
	h.testSecrets = map[string]string{}
	if h.testLicense != "" {
		bytes, err := ioutil.ReadFile(h.testLicense)
		if err != nil {
			return fmt.Errorf("reading %v: %w", h.testLicense, err)
		}
		h.testSecrets["test-license.json"] = string(bytes)
		h.testContext.TestLicense = "/var/run/secrets/e2e/test-license.json"
	}

	if h.testLicensePKeyPath != "" {
		bytes, err := ioutil.ReadFile(h.testLicensePKeyPath)
		if err != nil {
			return fmt.Errorf("reading %v: %w", h.testLicensePKeyPath, err)
		}
		h.testSecrets["dev-private.key"] = string(bytes)
		h.testContext.TestLicensePKeyPath = "/var/run/secrets/e2e/dev-private.key"
	}

	if h.monitoringSecrets != "" {
		bytes, err := ioutil.ReadFile(h.monitoringSecrets)
		if err != nil {
			return fmt.Errorf("reading %v: %w", h.monitoringSecrets, err)
		}

		monitoringSecrets := struct {
			MonitoringIP   string `json:"monitoring_ip"`
			MonitoringUser string `json:"monitoring_user"`
			MonitoringPass string `json:"monitoring_pass"`
			MonitoringCA   string `json:"monitoring_ca"`
			APMServerCert  string `json:"apm_server_cert"`
			APMSecretToken string `json:"apm_secret_token"`
			APMServerURL   string `json:"apm_server_url"`
		}{}

		if err := json.Unmarshal(bytes, &monitoringSecrets); err != nil {
			return fmt.Errorf("unmarshal %v, %w", h.monitoringSecrets, err)
		}

		h.testSecrets["monitoring_ip"] = monitoringSecrets.MonitoringIP
		h.testSecrets["monitoring_user"] = monitoringSecrets.MonitoringUser
		h.testSecrets["monitoring_pass"] = monitoringSecrets.MonitoringPass
		h.testSecrets["monitoring_ca"] = monitoringSecrets.MonitoringCA

		h.operatorSecrets = map[string]string{}
		h.operatorSecrets["apm_server_cert"] = monitoringSecrets.APMServerCert
		h.operatorSecrets["apm_secret_token"] = monitoringSecrets.APMSecretToken
		h.operatorSecrets["apm_server_url"] = monitoringSecrets.APMServerURL
	}

	return nil
}

func (h *helper) createE2ENamespaceAndRoleBindings() error {
	log.Info("Creating E2E namespace and role bindings")
	return h.kubectlApplyTemplateWithCleanup("config/e2e/rbac.yaml", h.testContext)
}

func (h *helper) createRoles() error {
	log.Info("Creating Beat/Agent roles")
	return h.kubectlApplyTemplateWithCleanup("config/e2e/roles.yaml", h.testContext)
}

func (h *helper) installCRDs() error {
	log.Info("Installing CRDs")
	_, err := h.kubectl("apply", "-f", "config/crds/all-crds.yaml")
	return err
}

func (h *helper) createOperatorNamespaces() error {
	log.Info("Creating operator namespaces")
	return h.kubectlApplyTemplateWithCleanup("config/e2e/operator_namespaces.yaml", h.testContext)
}

func (h *helper) createManagedNamespaces() error {
	log.Info("Creating managed namespaces")
	// when in local mode, don't delete the namespaces on exit
	if h.testContext.Local {
		_, err := h.kubectlApplyTemplate("config/e2e/managed_namespaces.yaml", h.testContext)
		return err
	}

	return h.kubectlApplyTemplateWithCleanup("config/e2e/managed_namespaces.yaml", h.testContext)
}

func (h *helper) deployOperator() error {
	log.Info("Deploying operator")
	return h.kubectlApplyTemplateWithCleanup("config/e2e/operator.yaml", h.testContext)
}

func (h *helper) waitForOperatorToBeReady() error {
	log.Info("Waiting for the operator pod to be ready")
	client, err := h.createKubeClient()
	if err != nil {
		return errors.Wrap(err, "failed to create kubernetes client")
	}

	// operator pod name takes the form <statefulset name>-<ordinal>
	podName := fmt.Sprintf("%s-0", h.testContext.Operator.Name)

	return retry.UntilSuccess(func() error {
		pod, err := client.CoreV1().Pods(h.testContext.Operator.Namespace).Get(context.Background(), podName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !k8s.IsPodReady(*pod) {
			return fmt.Errorf("operator pod `%s` not ready", podName)
		}
		return nil
	}, operatorReadyTimeout, 10*time.Second)
}

func (h *helper) deploySecurityConstraints() error {
	if !h.testContext.OcpCluster {
		return nil
	}
	log.Info("Deploying SCC")
	return h.kubectlApplyTemplateWithCleanup("config/e2e/scc.yaml", h.testContext)
}

func (h *helper) deployMonitoring() error {
	if h.monitoringSecrets == "" {
		log.Info("No monitoring secrets provided, monitoring is not deployed")
		return nil
	}

	log.Info("Deploying monitoring")
	return h.kubectlApplyTemplateWithCleanup("config/e2e/monitoring.yaml", h.testContext)
}

func (h *helper) deployTestSecrets() error {
	log.Info("Deploying e2e test secret")
	return h.kubectlApplyTemplateWithCleanup("config/e2e/secrets.yaml",
		struct {
			Secrets         map[string]string
			OperatorSecrets map[string]string
			Context         test.Context
		}{
			Secrets:         h.testSecrets,
			OperatorSecrets: h.operatorSecrets,
			Context:         h.testContext,
		},
	)
}

func (h *helper) runTestJob() error {
	client, err := h.createKubeClient()
	if err != nil {
		return errors.Wrap(err, "failed to create kubernetes client")
	}

	// start the event logger to log all relevant events in the cluster
	stopChan := make(chan struct{})
	eventLogger := newEventLogger(client, h.testContext, h.eventLog)
	go eventLogger.Start(stopChan)

	// stream the logs while waiting for the test job to finish
	err = h.startAndMonitorTestJobs(client)
	close(stopChan)

	if err != nil {
		h.dumpEventLog()
		h.dumpK8sData()
		return errors.Wrap(err, "test run failed")
	}

	return nil
}

func (h *helper) createKubeClient() (*kubernetes.Clientset, error) {
	// load kubernetes client config
	var overrides clientcmd.ConfigOverrides
	var clientConfig clientcmd.ClientConfig

	if h.kubeConfig != "" {
		kubeConf, err := clientcmd.LoadFromFile(h.kubeConfig)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load kubeconfig")
		}

		clientConfig = clientcmd.NewDefaultClientConfig(*kubeConf, &overrides)
	} else {
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		clientConfig = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &overrides)
	}

	// create the kubernetes API client
	config, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create kubernetes client config")
	}

	return kubernetes.NewForConfig(config)
}

// monitorTestJob keeps track of the test pod to determine whether the tests failed or not.
func (h *helper) startAndMonitorTestJobs(client *kubernetes.Clientset) error {

	testSession := NewJobsManager(client, h)

	outputs := []io.Writer{os.Stdout}
	if h.logToFile {
		jl, err := newJSONLogToFile(testsLogFile)
		if err != nil {
			log.Error(err, "Failed to create log file for test output")
			return err
		}
		defer jl.Close()
		outputs = append(outputs, jl)
	}
	writer := io.MultiWriter(outputs...)
	runJob := NewJob("eck-"+h.testRunName, "config/e2e/e2e_job.yaml", writer, goLangTestTimestampParser)

	if h.deployChaosJob {
		chaosJob := NewJob("chaos-"+h.testRunName, "config/e2e/chaos_job.yaml", os.Stdout, stdTimestampParser)
		runJob.WithDependency(chaosJob)
		testSession.Schedule(chaosJob)
	}

	testSession.Schedule(runJob)

	testSession.Start() // block until log streamers are done

	return testSession.err
}

type LogStreamProvider interface {
	fmt.Stringer
	NewLogStream() (io.ReadCloser, error)
}

type PodLogStreamProvider struct {
	client         *kubernetes.Clientset
	pod, namespace string
}

func (p PodLogStreamProvider) NewLogStream() (io.ReadCloser, error) {
	sinceSeconds := int64(60 * 5)
	opts := &corev1.PodLogOptions{
		Container:    "e2e",
		Follow:       true,
		SinceSeconds: &sinceSeconds,
		Timestamps:   false,
	}

	req := p.client.CoreV1().Pods(p.namespace).GetLogs(p.pod, opts)
	return req.Stream(context.Background())
}

func (p PodLogStreamProvider) String() string {
	return p.pod
}

type timestampExtractor func(line []byte) (time.Time, error)

func streamTestJobOutput(
	streamProvider LogStreamProvider,
	timestampExtractor timestampExtractor,
	writer io.Writer,
	streamErrors chan<- error,
	stop <-chan struct{},
) {
	// The log stream may end abruptly in some environments where the network isn't reliable.
	// Let's retry when that happens. To avoid duplicate log entries, keep track of the last
	// test log timestamp: we don't want to reprocess entries with a lower timestamp.
	// If the stream drops in between several logs that share the same timestamp, the second half will be lost.
	lastTimestamp := time.Time{}
	for {
		select {
		case <-stop:
			log.Info("Log stream stopped", "pod_name", streamProvider)
			return
		default:
			log.Info("Streaming pod logs", "pod_name", streamProvider)
			stream, err := streamProvider.NewLogStream()
			if err != nil {
				log.Error(err, "Streaming pod logs failed", "pod_name", streamProvider)
				streamErrors <- err
				continue // retry
			}
			defer stream.Close()

			pastPreviousLogStream := false
			scan := bufio.NewScanner(stream)
			for scan.Scan() {
				line := scan.Bytes()

				timestamp, err := timestampExtractor(line)
				if err != nil {
					streamErrors <- err
					continue
				}

				// don't re-write logs that have been already written in a previous log stream attempt
				if !pastPreviousLogStream && !timestamp.After(lastTimestamp) {
					continue
				}

				// new log to process
				pastPreviousLogStream = true
				lastTimestamp = timestamp
				if _, err := writer.Write([]byte(string(line) + "\n")); err != nil {
					streamErrors <- err
					return
				}
			}
			if err := scan.Err(); err != nil {
				log.Error(err, "Log stream ended", "pod_name", streamProvider)
			} else {
				log.Info("Log stream ended", "pod_name", streamProvider)
			}
			// retry
		}
	}
}

type GoLangJSONLogLine struct {
	Time string
}

// goLangTestTimestampParser extract the timestamp from a log issued by "go test ...", it is expected that the line is well formatted jsonline
func goLangTestTimestampParser(line []byte) (time.Time, error) {
	var logLine GoLangJSONLogLine
	if err := json.Unmarshal(line, &logLine); err != nil {
		return time.Time{}, err
	}
	timestamp, err := time.Parse(time.RFC3339Nano, logLine.Time)
	if err != nil {
		return time.Time{}, err
	}
	return timestamp, nil
}

type StdJSONLogLine struct {
	Time string `json:"@timestamp"`
}

// stdTimestampParser extract the timestamp from a log issued by "go test ...", it is expected that the line is well formatted jsonline
func stdTimestampParser(line []byte) (time.Time, error) {
	var logLine StdJSONLogLine
	if err := json.Unmarshal(line, &logLine); err != nil {
		return time.Time{}, err
	}
	timestamp, err := time.Parse(time.RFC3339Nano, logLine.Time)
	if err != nil {
		return time.Time{}, err
	}
	return timestamp, nil
}

func (h *helper) kubectlApplyTemplate(templatePath string, templateParam interface{}) (string, error) {
	outFilePath, err := h.renderTemplate(templatePath, templateParam)
	if err != nil {
		return "", err
	}

	_, err = h.kubectl("apply", "-f", outFilePath)
	return outFilePath, err
}

func (h *helper) kubectlApplyTemplateWithCleanup(templatePath string, templateParam interface{}) error {
	resourceFile, err := h.kubectlApplyTemplate(templatePath, templateParam)
	if err != nil {
		return err
	}

	h.addCleanupFunc(h.deleteResources(resourceFile))
	return nil
}

func (h *helper) kubectl(command string, args ...string) (string, error) {
	return h.exec(h.kubectlWrapper.Command(command, args...))
}

func (h *helper) exec(cmd *command.Command) (string, error) {
	ctx, cancelFunc := context.WithTimeout(context.Background(), h.commandTimeout)
	defer cancelFunc()

	log.V(1).Info("Executing command", "command", cmd)
	out, err := cmd.Execute(ctx)
	outString := string(out)
	if err != nil {
		// suppress the stacktrace when the command fails naturally
		if _, ok := err.(*exec.ExitError); ok {
			log.Info("Command returned error code", "command", cmd, "message", err.Error())
		} else {
			log.Error(err, "Command execution failed", "command", cmd)
		}

		fmt.Fprintln(os.Stderr, outString)
		return "", errors.Wrapf(err, "command failed: [%s]", cmd)
	}

	if log.V(1).Enabled() {
		fmt.Println(outString)
	}

	return outString, nil
}

func (h *helper) renderTemplate(templatePath string, param interface{}) (string, error) {
	tmpl, err := template.New(filepath.Base(templatePath)).Funcs(sprig.TxtFuncMap()).ParseFiles(templatePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse template at %s", templatePath)
	}

	// to avoid creating subdirectories, convert the file path to a flattened path
	// Eg. path/to/config.yaml will become path_to_config.yaml
	outFilePath := filepath.Join(h.scratchDir, strings.Replace(templatePath, string(filepath.Separator), "_", -1))
	f, err := os.Create(outFilePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create file: %s", outFilePath)
	}

	defer f.Close()
	if err := tmpl.Execute(f, param); err != nil {
		return "", errors.Wrapf(err, "failed to render template to %s", outFilePath)
	}

	return outFilePath, nil
}

func (h *helper) deleteResources(file string) func() {
	return func() {
		log.Info("Deleting resources", "file", file)
		if _, err := h.kubectl("delete", "--all", "--wait", "-f", file); err != nil {
			log.Error(err, "Failed to delete resources", "file", file)
		}
	}
}

func (h *helper) addCleanupFunc(cf func()) {
	h.cleanupFuncs = append(h.cleanupFuncs, cf)
}

func (h *helper) runCleanup() {
	if h.skipCleanup {
		log.Info("Skipping cleanup")
		return
	}

	// run the cleanup functions in the reverse order they were added
	for i := len(h.cleanupFuncs) - 1; i >= 0; i-- {
		cf := h.cleanupFuncs[i]
		cf()
	}
}

func (h *helper) dumpEventLog() {
	f, err := os.Open(h.eventLog)
	if err != nil {
		log.Error(err, "Failed to open event log", "path", h.eventLog)
		return
	}

	log.Info("Cluster event log")
	var buffer [1024]byte
	if _, err := io.CopyBuffer(os.Stdout, f, buffer[:]); err != nil {
		log.Error(err, "Failed to output event log")
	}
}

func (h *helper) dumpK8sData() {
	operatorNs := h.testContext.Operator.Namespace
	managedNs := strings.Join(h.testContext.Operator.ManagedNamespaces, ",")
	cmd := exec.Command("hack/diagnostics/eck-dump.sh", "-N", operatorNs, "-n", managedNs, "-o", h.testContext.TestRun, "-z")
	err := cmd.Run()
	if err != nil {
		log.Error(err, "Failed to run hack/diagnostics/eck-dump.sh")
	}
}
