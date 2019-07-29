// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/command"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	jobTimeout       = 120 * time.Minute // time to wait for the test job to finish
	kubePollInterval = 10 * time.Second  // Kube API polling interval
	logBufferSize    = 1024              // Size of the log buffer (1KiB)
	testRunLabel     = "test-run"        // name of the label applied to resources
)

func doRun(flags runFlags) error {
	helper := &helper{
		runFlags:       flags,
		kubectlWrapper: command.NewKubectl(flags.kubeConfig),
	}

	steps := []func() error{
		helper.createTestOutDir,
		helper.initTestContext,
		helper.createE2ENamespaceAndRoleBindings,
		helper.installCRDs,
		helper.deployGlobalOperator,
		helper.deployNamespaceOperators,
		helper.deployTestJob,
		helper.runTestJob,
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
	eventLog       string
	kubectlWrapper *command.Kubectl
	testContext    test.Context
	testOutDir     string
	cleanupFuncs   []func()
}

func (h *helper) createTestOutDir() error {
	h.testOutDir = filepath.Join(h.testOutDirRoot, h.testRunName)
	log.Info("Creating test output directory", "directory", h.testOutDir)

	// ensure that the directory does not exist
	if _, err := os.Stat(h.testOutDir); !os.IsNotExist(err) {
		return errors.Wrapf(err, "test output directory already exists: %s", h.testOutDir)
	}

	// create the directory
	if err := os.MkdirAll(h.testOutDir, os.ModePerm); err != nil {
		return errors.Wrapf(err, "failed to create test output directory: %s", h.testOutDir)
	}

	// generate the path to the event log
	h.eventLog = filepath.Join(h.testOutDir, "event.log")

	// clean up the directory
	h.addCleanupFunc(func() {
		log.Info("Cleaning up the test output directory", "directory", h.testOutDir)
		if err := os.RemoveAll(h.testOutDir); err != nil {
			log.Error(err, "Failed to cleanup test output directory", "path", h.testOutDir)
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
		GlobalOperator: test.ClusterResource{
			Name:      fmt.Sprintf("%s-global-operator", h.testRunName),
			Namespace: fmt.Sprintf("%s-elastic-system", h.testRunName),
		},
		NamespaceOperators: make([]test.NamespaceOperator, len(h.managedNamespaces)),
		OperatorImage:      h.operatorImage,
		TestLicence:        h.testLicence,
		TestRegex:          h.testRegex,
		TestRun:            h.testRunName,
	}

	for i, ns := range h.managedNamespaces {
		h.testContext.NamespaceOperators[i] = test.NamespaceOperator{
			ClusterResource: test.ClusterResource{
				Name:      fmt.Sprintf("%s-%s-ns-operator", h.testRunName, ns),
				Namespace: fmt.Sprintf("%s-ns-operators", h.testRunName),
			},
			ManagedNamespace: fmt.Sprintf("%s-%s", h.testRunName, ns),
		}
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

func (h *helper) createE2ENamespaceAndRoleBindings() error {
	log.Info("Creating E2E namespace and role bindings")
	return h.kubectlApplyTemplate("config/e2e/rbac.yaml", h.testContext, true)
}

func (h *helper) installCRDs() error {
	log.Info("Installing CRDs")
	crds, err := filepath.Glob("config/crds/*.yaml")
	if err != nil {
		return errors.Wrap(err, "failed to list CRDs")
	}

	for _, crd := range crds {
		log.V(2).Info("Installing CRD", "crd", crd)
		if err := h.kubectlApplyTemplate(crd, h.testContext, false); err != nil {
			return err
		}
	}

	return nil
}

func (h *helper) deployGlobalOperator() error {
	log.Info("Deploying global operator")
	return h.kubectlApplyTemplate("config/e2e/global_operator.yaml", h.testContext, true)
}

func (h *helper) deployNamespaceOperators() error {
	log.Info("Deploying namespace operators")
	return h.kubectlApplyTemplate("config/e2e/namespace_operator.yaml", h.testContext, true)
}

func (h *helper) deployTestJob() error {
	log.Info("Deploying e2e test job")
	return h.kubectlApplyTemplate("config/e2e/batch_job.yaml", h.testContext, true)
}

func (h *helper) runTestJob() error {
	if h.setupOnly {
		log.Info("Skipping tests because this is a setup-only run")
		return nil
	}

	client, err := h.createKubeClient()
	if err != nil {
		return errors.Wrap(err, "failed to create kubernetes client")
	}

	// start the event logger to log all relevant events in the cluster
	stopChan := make(chan struct{})
	eventLogger := newEventLogger(client, h.testContext, h.eventLog)
	go eventLogger.Start(stopChan)

	// stream the logs while waiting for the test job to finish
	err = h.monitorTestJob(client)
	close(stopChan)

	if err != nil {
		h.dumpEventLog()
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
func (h *helper) monitorTestJob(client *kubernetes.Clientset) error {
	log.Info("Waiting for test job to start")
	ctx, cancelFunc := context.WithTimeout(context.Background(), jobTimeout)
	defer func() {
		cancelFunc()
		runtime.HandleCrash()
	}()

	factory := informers.NewSharedInformerFactoryWithOptions(client, kubePollInterval,
		informers.WithNamespace(h.testContext.E2ENamespace),
		informers.WithTweakListOptions(func(opt *metav1.ListOptions) {
			opt.LabelSelector = fmt.Sprintf("%s=%s", testRunLabel, h.testContext.TestRun)
		}))

	informer := factory.Core().V1().Pods().Informer()

	jobStarted := false
	streamStatus := make(chan error, 1)
	var err error

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			log.Info("Pod added", "name", pod.Name)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			newPod := newObj.(*corev1.Pod)
			switch newPod.Status.Phase {
			case corev1.PodRunning:
				if !jobStarted {
					jobStarted = true
					log.Info("Pod started", "name", newPod.Name)
					go h.streamTestJobOutput(streamStatus, client, newPod.Name)
				} else {
					select {
					case streamErr := <-streamStatus:
						if streamErr != nil {
							log.Error(streamErr, "Stream failure")
							err = streamErr
							cancelFunc()
						}
					default:
					}
				}
			case corev1.PodSucceeded:
				log.Info("Tests completed successfully", "name", newPod.Name)
				cancelFunc()
			case corev1.PodFailed:
				log.Info("Pod is in failed state", "name", newPod.Name)
				err = errors.New("tests failed")
				cancelFunc()
			default:
				log.Info("Waiting for pod to be ready", "name", newPod.Name, "status", newPod.Status.Phase)
			}
		},
		DeleteFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			log.Info("Pod deleted", "name", pod.Name)
			cancelFunc()
		},
	})

	informer.Run(ctx.Done())
	return err
}

func (h *helper) streamTestJobOutput(streamStatus chan<- error, client *kubernetes.Clientset, pod string) {
	log.Info("Streaming pod logs", "name", pod)
	defer close(streamStatus)
	sinceSeconds := int64(30)
	opts := &corev1.PodLogOptions{
		Container:    "e2e",
		Follow:       true,
		SinceSeconds: &sinceSeconds,
	}

	req := client.CoreV1().Pods(h.testContext.E2ENamespace).GetLogs(pod, opts).Context(context.Background())
	stream, err := req.Stream()
	if err != nil {
		streamStatus <- err
		return
	}
	defer stream.Close()

	var buffer [logBufferSize]byte
	if _, err := io.CopyBuffer(os.Stdout, stream, buffer[:]); err != nil {
		if err == io.EOF {
			log.Info("Log stream ended")
			return
		}

		streamStatus <- err
	}
}

func (h *helper) kubectlApplyTemplate(templatePath string, templateParam interface{}, deleteOnExit bool) error {
	outFilePath, err := h.renderTemplate(templatePath, templateParam)
	if err != nil {
		return err
	}

	if err := h.kubectl("apply", "-f", outFilePath); err != nil {
		return err
	}

	if deleteOnExit {
		h.addCleanupFunc(func() {
			log.Info("Deleting resources", "file", outFilePath)
			if err := h.kubectl("delete", "--all", "-f", outFilePath); err != nil {
				log.Error(err, "Failed to delete resources", "file", outFilePath)
			}
		})
	}

	return nil
}

func (h *helper) kubectl(command string, args ...string) error {
	return h.exec(h.kubectlWrapper.Command(command, args...))
}

func (h *helper) exec(cmd *command.Command) error {
	ctx, cancelFunc := context.WithTimeout(context.Background(), h.commandTimeout)
	defer cancelFunc()

	log.V(1).Info("Executing command", "command", cmd)
	out, err := cmd.Execute(ctx)
	if err != nil {
		// suppress the stacktrace when the command fails naturally
		if _, ok := err.(*exec.ExitError); ok {
			log.Info("Command returned error code", "command", cmd, "message", err.Error())
		} else {
			log.Error(err, "Command execution failed", "command", cmd)
		}

		fmt.Fprintln(os.Stderr, string(out))
		return errors.Wrapf(err, "command failed: [%s]", cmd)
	}

	if log.V(1).Enabled() {
		fmt.Println(string(out))
	}

	return nil
}

func (h *helper) renderTemplate(templatePath string, param interface{}) (string, error) {
	tmpl, err := template.New(filepath.Base(templatePath)).Funcs(sprig.TxtFuncMap()).ParseFiles(templatePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse template at %s", templatePath)
	}

	outFilePath := filepath.Join(h.testOutDir, strings.Replace(templatePath, string(filepath.Separator), "_", -1))
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
