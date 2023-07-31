// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

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
	runningWg  *sync.WaitGroup // wait for the dependency to be started

	// Job context
	jobStarted    bool // keep track of the first Pod running event
	stopRequested bool // keep track of the stop attempts

	// Job logs management
	timestampExtractor timestampExtractor
	streamErrors       chan error      // receive log stream errors
	stopLogStream      chan struct{}   // notify the log stream it can stop when EOF
	logStreamWg        *sync.WaitGroup // wait for the log stream goroutines to be over
	writer             io.Writer       // where to stream the logs in "realtime"

	// Job artefacts directory that contains files to download when the pod is ready
	artefactsDir        string
	artefactsDownloaded bool // keep track of download attempts
}

func NewJob(podName, templatePath string, writer io.Writer, timestampExtractor timestampExtractor) *Job {
	logStreamWg := &sync.WaitGroup{}
	runningWg := &sync.WaitGroup{}
	runningWg.Add(1)
	return &Job{
		jobName:            podName,
		templatePath:       templatePath,
		jobStarted:         false,
		stopLogStream:      make(chan struct{}), // notify the log stream it can stop when EOF
		streamErrors:       make(chan error, 1), // receive log stream errors
		writer:             writer,
		runningWg:          runningWg,
		logStreamWg:        logStreamWg,
		timestampExtractor: timestampExtractor,
	}
}

// Stop is only a best effort to stop the streaming process
func (j *Job) Stop() {
	if j.stopRequested {
		return
	}
	j.stopRequested = true
	close(j.stopLogStream)
}

func (j *Job) WithDependency(dependency *Job) *Job {
	j.dependency = dependency
	return j
}

func (j *Job) WithArtefactsDir(d string) *Job {
	j.artefactsDir = d
	return j
}

// onPodEvent ensures that log streaming is started and also manages the internal state of the Job based on the events
// received from the informer.
func (j *Job) onPodEvent(client *kubernetes.Clientset, pod *corev1.Pod) {
	if pod.Status.Phase == corev1.PodRunning && !j.jobStarted {
		j.jobStarted = true
		j.runningWg.Done() // notify dependent that this job has started
		log.Info("Pod started", "namespace", pod.Namespace, "name", pod.Name)
		j.logStreamWg.Add(1)
		go func() {
			// Read stream failure errors
			for streamErr := range j.streamErrors {
				log.Info("Stream failure", "pod", pod.Name, "err", streamErr)
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
}
