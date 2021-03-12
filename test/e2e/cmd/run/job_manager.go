// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package run

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// JobsManager runs and monitors one or more jobs on a remote K8S cluster.
type JobsManager struct {
	*helper
	cache.SharedInformer
	context.Context
	cancelFunc context.CancelFunc
	*kubernetes.Clientset

	jobs map[string]Job
	err  error // used to notify that an error occurred in this session
}

func NewJobsManager(client *kubernetes.Clientset, h *helper, podSelector string, timeout time.Duration) *JobsManager {
	factory := informers.NewSharedInformerFactoryWithOptions(client, kubePollInterval,
		informers.WithTweakListOptions(func(opt *metav1.ListOptions) {
			opt.LabelSelector = fmt.Sprintf(
				"%s=%s,%s=%v",
				testRunLabel,
				h.testContext.TestRun,
				podSelector,
				true,
			)
			log.Info("Informer configured", "label-selector", opt.LabelSelector)
		}))
	ctx, cancelFunc := context.WithTimeout(context.Background(), timeout)
	return &JobsManager{
		helper:         h,
		Clientset:      client,
		SharedInformer: factory.Core().V1().Pods().Informer(),
		Context:        ctx,
		jobs:           map[string]Job{},
		cancelFunc:     cancelFunc,
	}
}

// Schedule schedules some Jobs. A Job is started only when its dependency is fulfilled (if any).
func (jm *JobsManager) Schedule(jobs ...Job) {
	for _, job := range jobs {
		j := job
		jm.jobs[j.Name()] = j
		go func() {
			// Check if dependency is started
			if dep, ok := j.(JobDependency); ok {
				dep.WaitOnRunning()
			}
			// Check if context is still valid
			if jm.Err() != nil {
				j.Skip()
				return
			}
			// Create the Job
			log.Info("Creating job", "job_name", j.Name())
			err := j.Materialize()
			if err != nil {
				log.Error(err, "Error while creating Job", "job_name", j.Name())
				jm.err = err
				jm.Stop()
			}
		}()
	}
}

// Start starts the informer, forwards the events to the Jobs and attempts to stop and return as soon as a first
// Job is completed, either because it has succeeded of failed.
func (jm *JobsManager) Start() {
	log.Info("Starting to monitor jobs")

	defer func() {
		jm.cancelFunc()
		if deadline, _ := jm.Deadline(); deadline.Before(time.Now()) {
			log.Info("Test job timeout exceeded", "timeout", testSessionTimeout)
		}
		runtime.HandleCrash()
	}()

	jm.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			log.Info("Pod added", "name", pod.Name)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			newPod := newObj.(*corev1.Pod)
			jobName, hasJobName := newPod.Labels["job-name"]
			if !hasJobName {
				// Unmanaged Job/Pod, this should not happen if the label selector is correct, harmless but report it in the logs.
				log.Info("received an update event for a Pod which was not created by a job", "namespace", newPod.Namespace, "name", newPod.Name)
				return
			}
			job, ok := jm.jobs[jobName]
			if !ok {
				// Same as above, it seems to be an unmanaged Job/Pod.
				log.Info("received an update event for an unmanaged Pod", "namespace", newPod.Namespace, "name", newPod.Name)
				return
			}
			switch newPod.Status.Phase {
			case corev1.PodRunning:
				job.OnPodEvent(jm.Clientset, newPod)
			case corev1.PodSucceeded:
				log.Info("Pod succeeded", "name", newPod.Name, "status", newPod.Status.Phase)
				job.OnPodEvent(jm.Clientset, newPod)
				jm.Stop()
			case corev1.PodFailed:
				// One of the managed Job/Pod has failed, wait for logs and return.
				jm.err = errors.Errorf("Pod %s has failed", newPod.Name)
				log.Info("Pod is in failed state", "name", newPod.Name, "err", jm.err, "status", newPod.Status)
				job.OnPodEvent(jm.Clientset, newPod)
				jm.Stop()
			default:
				log.Info("Waiting for pod to be ready", "name", newPod.Name, "status", newPod.Status.Phase)
			}
		},
		DeleteFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			log.Info("Pod deleted", "name", pod.Name)
			jm.Stop()
		},
	})
	jm.Run(jm.Done())
}

func (jm *JobsManager) Stop() {
	for _, job := range jm.jobs {
		job.Stop()
	}
	jm.cancelFunc()
}
