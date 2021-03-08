// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package run

import (
	"io"
	"os"
	"os/exec"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

type Job interface {
	Name() string
	Materialize() error
	OnPodEvent(*kubernetes.Clientset, *corev1.Pod)
	Skip()
	Stop()
}

type JobDependency interface {
	WaitOnRunning()
}

// LogProducingJob represents a task materialized by a Kubernetes Pod.
type LogProducingJob struct {
	jobName    string
	jobFactory func() error

	// Job dependency
	dependency *LogProducingJob
	runningWg  *sync.WaitGroup // wait for the dependency to be started

	// Job context
	jobStarted    bool // keep track of the first Pod running event
	stopRequested bool // keep track when a stop request has already been requested

	// Job logs management
	timestampExtractor timestampExtractor
	streamErrors       chan error      // receive log stream errors
	stopLogStream      chan struct{}   // notify the log stream it can stop when EOF
	logStreamWg        *sync.WaitGroup // wait for the log stream goroutines to be over
	writer             io.Writer       // where to stream the logs in "realtime"
}

var _ Job = &LogProducingJob{}

func NewJob(podName string, jobFactory func() error, writer io.Writer, timestampExtractor timestampExtractor) *LogProducingJob {
	logStreamWg := &sync.WaitGroup{}
	runningWg := &sync.WaitGroup{}
	runningWg.Add(1)
	return &LogProducingJob{
		jobName:            podName,
		jobFactory:         jobFactory,
		jobStarted:         false,
		stopLogStream:      make(chan struct{}), // notify the log stream it can stop when EOF
		streamErrors:       make(chan error, 1), // receive log stream errors
		writer:             writer,
		runningWg:          runningWg,
		logStreamWg:        logStreamWg,
		timestampExtractor: timestampExtractor,
	}
}

func (j *LogProducingJob) Materialize() error {
	return j.jobFactory()
}

func (j *LogProducingJob) Name() string {
	return j.jobName
}

// WaitForLogs waits for logs to be fully read before leaving.
func (j *LogProducingJob) OnPodTermination() {
	if j.stopRequested {
		// already done in the past
		return
	}
	j.stopRequested = true
	close(j.stopLogStream)
	log.Info("Waiting for log stream to be over", "name", j.jobName)
	j.logStreamWg.Wait()
	close(j.streamErrors)
}

func (j *LogProducingJob) Skip() {
	log.Info("Skip job creation", "job_name", j.jobName)
	j.runningWg.Done()
}

// Stop is only a best effort to stop the streaming process
func (j *LogProducingJob) Stop() {
	if j.stopRequested {
		// Job already stopped
		return
	}
	close(j.stopLogStream)
}

func (j *LogProducingJob) WithDependency(dependency *LogProducingJob) {
	j.dependency = dependency
}


func (j *LogProducingJob) WaitOnRunning() {
	if j.dependency != nil {
		log.Info("Waiting for dependency job to be started", "job_name", j.Name(), "dependency_name", j.dependency.Name())
		j.dependency.runningWg.Wait()
	}
}

// onPodEvent ensures that log streaming is started and also manages the internal state of the Job based on the events
// received from the informer.
func (j *LogProducingJob) OnPodEvent(client *kubernetes.Clientset, pod *corev1.Pod) {
	switch pod.Status.Phase {
	case corev1.PodRunning:
		if !j.jobStarted {
			j.jobStarted = true
			j.runningWg.Done() // notify dependent that this job has started
			log.Info("Pod started", "namespace", pod.Namespace, "name", pod.Name)
			j.logStreamWg.Add(1)
			go func() {
				// Read stream failure errors
				for streamErr := range j.streamErrors {
					log.Error(streamErr, "Stream failure")
				}
			}()
			go func() {
				streamProvider := &PodLogStreamProvider{
					client:    client,
					pod:       pod.Name,
					namespace: pod.Namespace,
				}
				streamTestJobOutput(streamProvider, j.timestampExtractor, j.writer, j.streamErrors, j.stopLogStream)
				defer j.logStreamWg.Done()
			}()
		}
	case corev1.PodFailed:
		j.OnPodTermination()
	case corev1.PodSucceeded:
		j.OnPodTermination()
	}
}

// ArtifactProducingJob represents a task materialized by a Kubernetes Pod.
type ArtifactProducingJob struct {
	jobName    string
	jobFactory func() error

	// Job context
	jobStarted    bool // keep track of the first Pod running event
	config *restclient.Config
}

var _ Job = &ArtifactProducingJob{}

func NewArtifactJob(name string, jobFactory func() error, cfg *restclient.Config) *ArtifactProducingJob {
	return &ArtifactProducingJob{
		jobName:      name,
		jobFactory:   jobFactory,
		jobStarted:   false,
		config:       cfg,
	}
}

func (j *ArtifactProducingJob) Materialize() error {
	return j.jobFactory()
}

func (j *ArtifactProducingJob) Name() string {
	return j.jobName
}


func (j *ArtifactProducingJob) Skip() {
	log.Info("Skip job creation", "job_name", j.jobName)
}

// Stop is only a best effort to stop the streaming process
func (j *ArtifactProducingJob) Stop() {}


// onPodEvent ensures that log streaming is started and also manages the internal state of the Job based on the events
// received from the informer.
func (j *ArtifactProducingJob) OnPodEvent(client *kubernetes.Clientset, pod *corev1.Pod) {
	switch pod.Status.Phase {
	case corev1.PodRunning:
		if !j.jobStarted {
			j.jobStarted = true
			log.Info("Pod started", "namespace", pod.Namespace, "name", pod.Name)
			go func() {
				req := client.CoreV1().RESTClient().Post().Resource("pods").Name(pod.Name).Namespace(pod.Namespace).
					SubResource("exec").VersionedParams(&corev1.PodExecOptions{
					Stdin:     false,
					Stdout:    true,
					Stderr:    true,
					TTY:       false,
					Container: "offer-output",
					Command:   []string{"sh", "-c", "cat /export-pipe"},
				}, scheme.ParameterCodec)
				executor, err := remotecommand.NewSPDYExecutor(j.config, "POST", req.URL())
				if err != nil {
					log.Error(err, "failed to exec in to pod", "namespace", pod.Namespace, "name", pod.Name)
					return
				}
				cmd := exec.Command("tar", "-xvf", "-")
				pipe, err := cmd.StdinPipe()
				if err != nil {
					log.Error(err, "failed to start tar pipe", "namespace", pod.Namespace, "name", pod.Name)
					return
				}
				go func() {
				 	log.Info("streaming exec result", "namespace", pod.Namespace, "name", pod.Name)
					defer pipe.Close()
					err = executor.Stream(remotecommand.StreamOptions{
						Stdin:  nil,
						Stdout: pipe,
						Stderr: os.Stderr,
					})
					if err != nil {
						log.Error(err, "failed to stream remote command", "namespace", pod.Namespace, "name", pod.Name)
					}
				}()
				log.Info("Running tar", "namespace", pod.Namespace, "name", pod.Name)
				out, err := cmd.CombinedOutput()
				if err != nil {
					log.Error(err, "tar command failed", "namespace", pod.Namespace, "name", pod.Name)

				}
				log.Info(string(out),"namespace", pod.Namespace, "name", pod.Name )
			}()
		}
	}
}
