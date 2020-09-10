// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package run

import (
	"io"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// Job represents a task materialized by a Kubernetes Pod.
type Job struct {
	jobName      string
	templatePath string

	// Job dependency
	dependency *Job
	running    *sync.WaitGroup // wait for the dependency to be started

	// Job context
	jobStarted    bool // keep track of the first Pod running event
	podSucceeded  bool // keep track of when we're done
	stopRequested bool // keep track when a stop request has already been requested

	// Job logs management
	timestampExtractor timestampExtractor
	streamErrors       chan error      // receive log stream errors
	stopLogStream      chan struct{}   // notify the log stream it can stop when EOF
	logStreamWg        *sync.WaitGroup // wait for the log stream goroutines to be over
	writer             io.Writer       // where to stream the logs in "realtime"
}

func NewJob(podName, templatePath string, writer io.Writer, timestampExtractor timestampExtractor) *Job {
	logStreamWg := &sync.WaitGroup{}
	runningWg := &sync.WaitGroup{}
	runningWg.Add(1)
	return &Job{
		jobName:            podName,
		templatePath:       templatePath,
		jobStarted:         false,
		podSucceeded:       false,
		stopLogStream:      make(chan struct{}), // notify the log stream it can stop when EOF
		streamErrors:       make(chan error, 1), // receive log stream errors
		writer:             writer,
		running:            runningWg,
		logStreamWg:        logStreamWg,
		timestampExtractor: timestampExtractor,
	}
}

// WaitForLogs waits for logs to be fully read before leaving.
func (j *Job) WaitForLogs() {
	j.stopRequested = true
	close(j.stopLogStream)
	log.Info("Waiting for log stream to be over", "name", j.jobName)
	j.logStreamWg.Wait()
	close(j.streamErrors)
}

// Stop is only a best effort to stop the streaming process
func (j *Job) Stop() {
	if j.stopRequested {
		// Job already stopped
		return
	}
	close(j.stopLogStream)
}

func (j *Job) WithDependency(dependency *Job) *Job {
	j.dependency = dependency
	return j
}

// onPodEvent ensures that log streaming is started and also manages the internal state of the Job based on the events
// received from the informer.
func (j *Job) onPodEvent(client *kubernetes.Clientset, pod *corev1.Pod) {
	switch pod.Status.Phase {
	case corev1.PodRunning:
		if !j.jobStarted {
			j.jobStarted = true
			j.running.Done() // notify dependent that this job has started
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
	case corev1.PodSucceeded:
		j.podSucceeded = true
	}
}
