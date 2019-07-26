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
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
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

	steps := []func(){
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
		if helper.err != nil {
			return helper.err
		}
		step()
	}

	return helper.err
}

type helper struct {
	runFlags
	err            error
	eventLog       string
	kubectlWrapper *command.Kubectl
	testContext    test.Context
	testOutDir     string
	cleanupFuncs   []func()
}

func (s *helper) createTestOutDir() {
	s.testOutDir = filepath.Join(s.testOutDirRoot, s.testRunName)
	log.Info("Creating test output directory", "directory", s.testOutDir)

	// ensure that the directory does not exist
	if _, err := os.Stat(s.testOutDir); !os.IsNotExist(err) {
		s.err = errors.Wrapf(err, "test output directory already exists: %s", s.testOutDir)
		return
	}

	// create the directory
	if err := os.MkdirAll(s.testOutDir, os.ModePerm); err != nil {
		s.err = errors.Wrapf(err, "failed to create test output directory: %s", s.testOutDir)
		return
	}

	// generate the path to the event log
	s.eventLog = filepath.Join(s.testOutDir, "event.log")

	// clean up the directory
	s.addCleanupFunc(func() {
		log.Info("Cleaning up the test output directory", "directory", s.testOutDir)
		if err := os.RemoveAll(s.testOutDir); err != nil {
			log.Error(err, "Failed to cleanup test output directory", "path", s.testOutDir)
		}
	})
}

func (s *helper) initTestContext() {
	s.testContext = test.Context{
		AutoPortForwarding:  s.autoPortForwarding,
		E2EImage:            s.e2eImage,
		E2ENamespace:        s.testRunName,
		E2EServiceAccount:   s.testRunName,
		ElasticStackVersion: s.elasticStackVersion,
		GlobalOperator: test.ClusterResource{
			Name:      fmt.Sprintf("%s-global-operator", s.testRunName),
			Namespace: fmt.Sprintf("%s-elastic-system", s.testRunName),
		},
		NamespaceOperators: make([]test.NamespaceOperator, len(s.managedNamespaces)),
		OperatorImage:      s.operatorImage,
		TestLicence:        s.testLicence,
		TestRegex:          s.testRegex,
		TestRun:            s.testRunName,
	}

	for i, ns := range s.managedNamespaces {
		s.testContext.NamespaceOperators[i] = test.NamespaceOperator{
			ClusterResource: test.ClusterResource{
				Name:      fmt.Sprintf("%s-%s-ns-operator", s.testRunName, ns),
				Namespace: fmt.Sprintf("%s-ns-operators", s.testRunName),
			},
			ManagedNamespace: fmt.Sprintf("%s-%s", s.testRunName, ns),
		}
	}

	// write the test context if required
	if s.testContextOutPath != "" {
		log.Info("Writing test context", "path", s.testContextOutPath)
		f, err := os.Create(s.testContextOutPath)
		if err != nil {
			s.err = errors.Wrap(err, "failed to write test context")
			return
		}

		defer f.Close()
		enc := json.NewEncoder(f)
		if err := enc.Encode(s.testContext); err != nil {
			s.err = errors.Wrap(err, "failed to encode test context")
		}
	}
}

func (s *helper) createE2ENamespaceAndRoleBindings() {
	log.Info("Creating E2E namespace and role bindings")
	s.kubectlApplyTemplate("config/e2e/rbac.yaml", s.testContext, true)
}

func (s *helper) installCRDs() {
	log.Info("Installing CRDs")
	crds, err := filepath.Glob("config/crds/*.yaml")
	if err != nil {
		s.err = errors.Wrap(err, "failed to list CRDs")
		return
	}

	for _, crd := range crds {
		log.V(2).Info("Installing CRD", "crd", crd)
		s.kubectlApplyTemplate(crd, s.testContext, false)
	}
}

func (s *helper) deployGlobalOperator() {
	log.Info("Deploying global operator")
	s.kubectlApplyTemplate("config/e2e/global_operator.yaml", s.testContext, true)
}

func (s *helper) deployNamespaceOperators() {
	log.Info("Deploying namespace operators")
	s.kubectlApplyTemplate("config/e2e/namespace_operator.yaml", s.testContext, true)
}

func (s *helper) deployTestJob() {
	log.Info("Deploying e2e test job")
	s.kubectlApplyTemplate("config/e2e/batch_job.yaml", s.testContext, true)
}

func (s *helper) runTestJob() {
	if s.setupOnly {
		log.Info("Skipping tests because this is a setup-only run")
		return
	}

	client, err := s.createKubeClient()
	if err != nil {
		s.err = errors.Wrap(err, "failed to create kubernetes client")
		return
	}

	// start the event logger to log all relevant events in the cluster
	stopChan := make(chan struct{})
	eventLogger := newEventLogger(client, s.testContext, s.eventLog)
	go eventLogger.Start(stopChan)

	// launch the test job and wait for it to finish
	err = s.monitorTestJob(client)
	close(stopChan)

	if err != nil {
		s.err = errors.Wrap(err, "test run failed")
		s.dumpEventLog()
	}
}

func (s *helper) createKubeClient() (*kubernetes.Clientset, error) {
	// load kubernetes client config
	var overrides clientcmd.ConfigOverrides
	var clientConfig clientcmd.ClientConfig

	if s.kubeConfig != "" {
		kubeConf, err := clientcmd.LoadFromFile(s.kubeConfig)
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
func (s *helper) monitorTestJob(client *kubernetes.Clientset) error {
	log.Info("Waiting for test job to start")
	ctx, cancelFunc := context.WithTimeout(context.Background(), jobTimeout)
	defer func() {
		cancelFunc()
		runtime.HandleCrash()
	}()

	factory := informers.NewSharedInformerFactoryWithOptions(client, kubePollInterval,
		informers.WithNamespace(s.testContext.E2ENamespace),
		informers.WithTweakListOptions(func(opt *metav1.ListOptions) {
			opt.LabelSelector = fmt.Sprintf("%s=%s", testRunLabel, s.testContext.TestRun)
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
					go s.streamTestJobOutput(streamStatus, client, newPod.Name)
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

func (s *helper) streamTestJobOutput(streamStatus chan<- error, client *kubernetes.Clientset, pod string) {
	log.Info("Streaming pod logs", "name", pod)
	defer close(streamStatus)
	sinceSeconds := int64(30)
	opts := &corev1.PodLogOptions{
		Container:    "e2e", //TODO remove hardcoded value
		Follow:       true,
		SinceSeconds: &sinceSeconds,
	}

	req := client.CoreV1().Pods(s.testContext.E2ENamespace).GetLogs(pod, opts).Context(context.Background())
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

func (s *helper) kubectlApplyTemplate(templatePath string, templateParam interface{}, deleteOnExit bool) {
	if s.err != nil {
		return
	}

	outFilePath, err := s.renderTemplate(templatePath, templateParam)
	if err != nil {
		return
	}

	s.kubectl("apply", "-f", outFilePath)

	if deleteOnExit {
		s.addCleanupFunc(func() {
			log.Info("Deleting resources", "file", outFilePath)
			s.kubectl("delete", "--all", "-f", outFilePath)
		})
	}
}

func (s *helper) kubectl(command string, args ...string) {
	s.exec(s.kubectlWrapper.Command(command, args...))
}

func (s *helper) exec(cmd *command.Command) {
	ctx, cancelFunc := context.WithTimeout(context.Background(), s.commandTimeout)
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
		s.err = errors.Wrapf(err, "command failed: [%s]", cmd)
	}

	if log.V(1).Enabled() {
		fmt.Println(string(out))
	}
}

func (s *helper) renderTemplate(templatePath string, param interface{}) (string, error) {
	tmpl, err := template.New(filepath.Base(templatePath)).Funcs(sprig.TxtFuncMap()).ParseFiles(templatePath)
	if err != nil {
		s.err = errors.Wrapf(err, "failed to parse template at %s", templatePath)
		return "", err
	}

	outFilePath := filepath.Join(s.testOutDir, strings.Replace(templatePath, "/", "_", -1))
	f, err := os.Create(outFilePath)
	if err != nil {
		s.err = errors.Wrapf(err, "failed to create file: %s", outFilePath)
		return "", err
	}

	defer f.Close()
	if err := tmpl.Execute(f, param); err != nil {
		s.err = errors.Wrapf(err, "failed to render template to %s", outFilePath)
		return "", err
	}

	return outFilePath, nil
}

func (s *helper) addCleanupFunc(cf func()) {
	s.cleanupFuncs = append(s.cleanupFuncs, cf)
}

func (s *helper) runCleanup() {
	if s.skipCleanup {
		log.Info("Skipping cleanup")
		return
	}

	// run the cleanup functions in the reverse order they were added
	for i := len(s.cleanupFuncs) - 1; i >= 0; i-- {
		cf := s.cleanupFuncs[i]
		cf()
	}
}

func (s *helper) dumpEventLog() {
	f, err := os.Open(s.eventLog)
	if err != nil {
		log.Error(err, "Failed to open event log", "path", s.eventLog)
		return
	}

	log.Info("Cluster event log")
	var buffer [1024]byte
	if _, err := io.CopyBuffer(os.Stdout, f, buffer[:]); err != nil {
		log.Error(err, "Failed to output event log")
	}
}

type eventLogEntry struct {
	Message   string        `json:"message"`
	Kind      string        `json:"kind"`
	Name      string        `json:"name"`
	Namespace string        `json:"namespace"`
	RawEvent  *corev1.Event `json:"raw_event"`
}

type eventLogger struct {
	eventInformer         cache.SharedIndexInformer
	eventQueue            workqueue.RateLimitingInterface
	interestingNamespaces map[string]struct{}
	logFilePath           string
}

func newEventLogger(client *kubernetes.Clientset, testCtx test.Context, logFilePath string) *eventLogger {
	eventWatch := cache.NewListWatchFromClient(client.CoreV1().RESTClient(), "events", metav1.NamespaceAll, fields.Everything())
	el := &eventLogger{
		eventInformer:         cache.NewSharedIndexInformer(eventWatch, &corev1.Event{}, kubePollInterval, cache.Indexers{}),
		eventQueue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "elastic_co_eck_e2e_events"),
		interestingNamespaces: make(map[string]struct{}),
		logFilePath:           logFilePath,
	}

	el.eventInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if key, err := cache.MetaNamespaceKeyFunc(obj); err == nil {
				el.eventQueue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj); err == nil {
				el.eventQueue.Add(key)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if key, err := cache.MetaNamespaceKeyFunc(newObj); err == nil {
				el.eventQueue.Add(key)
			}
		},
	})

	// add all namespaces to intereting namespaces
	s := struct{}{}
	el.interestingNamespaces[testCtx.E2ENamespace] = s
	el.interestingNamespaces[testCtx.GlobalOperator.Namespace] = s
	for _, ns := range testCtx.NamespaceOperators {
		el.interestingNamespaces[ns.Namespace] = s
		el.interestingNamespaces[ns.ManagedNamespace] = s
	}

	return el
}

func (el *eventLogger) Start(stopChan <-chan struct{}) {
	defer func() {
		el.eventQueue.ShutDown()
		runtime.HandleCrash()
	}()

	go el.eventInformer.Run(stopChan)

	if !cache.WaitForCacheSync(stopChan, el.eventInformer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for cache to sync"))
		return
	}

	wait.Until(el.runEventProcessor, time.Second, stopChan)
}

func (el *eventLogger) runEventProcessor() {
	logFile, err := os.Create(el.logFilePath)
	if err != nil {
		log.Error(err, "Failed to create event log", "path", el.logFilePath)
		return
	}
	defer logFile.Close()
	logWriter := json.NewEncoder(logFile)

	for {
		key, quit := el.eventQueue.Get()
		if quit {
			return
		}

		evtObj, exists, err := el.eventInformer.GetIndexer().GetByKey(key.(string))
		if err != nil {
			log.Error(err, "failed to get event", "key", key)
			return
		}

		if !exists {
			continue
		}

		evt := evtObj.(*corev1.Event)
		if el.isInterestingEvent(evt) {
			logEntry := eventLogEntry{
				Message:   evt.Message,
				Kind:      evt.InvolvedObject.Kind,
				Name:      evt.InvolvedObject.Name,
				Namespace: evt.InvolvedObject.Namespace,
				RawEvent:  evt,
			}
			if err := logWriter.Encode(logEntry); err != nil {
				log.Error(err, "Failed to write event to event log", "event", evt)
			}
		}
	}
}

// isInterestingEvent determines whether an event is worthy of logging.
func (el *eventLogger) isInterestingEvent(evt *corev1.Event) bool {
	// was the event generated by an object in one of the namespaces associated with this test run?
	if _, exists := el.interestingNamespaces[evt.InvolvedObject.Namespace]; exists {
		// if the event is a warning, it should be logged
		return evt.Type != corev1.EventTypeNormal
	}
	return false
}
