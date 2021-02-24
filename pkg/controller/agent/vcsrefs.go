// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var knownRefs = map[string]string{}

func specImageOrDefault(spec v1alpha1.AgentSpec) string {
	if spec.Image == "" {
		return container.ImageRepository(container.AgentImage, spec.Version)
	}
	return spec.Image
}

func reconcileVCSRefs(params *Params, clientsetFactory func() (*kubernetes.Clientset, error)) *reconciler.Results {
	defer tracing.Span(&params.Context)

	requeueResult := reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
	res := reconciler.NewResult(params.Context)

	version := params.Agent.Spec.Version
	if _, ok := knownRefs[version]; ok {
		params.AgentVCSRef = knownRefs[version]
		params.Logger().V(1).Info("No agent inspection needed", "version", params.Agent.Spec.Version)
		return res
	}

	// TODO not all versions might result in valid pod names
	podName := types.NamespacedName{Name: fmt.Sprintf("elastic-agent-%s", version), Namespace: params.Agent.Namespace}
	containerName := "agent-inspector"
	pod := corev1.Pod{
		ObjectMeta: k8s.ToObjectMeta(podName),
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    containerName,
					Image:   specImageOrDefault(params.Agent.Spec),
					Command: []string{"ls", "/usr/share/elastic-agent/data"},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	err := params.Client.Get(params.Context, podName, &pod)
	if err != nil && apierrors.IsNotFound(err) {
		params.Logger().V(1).Info("Creating agent inspector pod", "version", params.Agent.Spec.Version)
		if err := params.Client.Create(params.Context, &pod); err != nil {
			return res.WithError(err)
		}
		// requeue to await job completion
		return res.WithResult(requeueResult)
	} else if err != nil {
		return res.WithError(err)
	}

	params.Logger().V(1).Info("Found agent inspector pod", "version", params.Agent.Spec.Version)
	switch pod.Status.Phase {
	case corev1.PodSucceeded:
		clientset, err := clientsetFactory()
		if err != nil {
			return res.WithError(err)
		}

		logOptions := corev1.PodLogOptions{
			Container: containerName,
			Follow:    false,
			Previous:  false,
		}
		raw, err := clientset.CoreV1().Pods(podName.Namespace).GetLogs(pod.Name, &logOptions).Do(params.Context).Raw()
		if err != nil {
			return res.WithError(err)
		}

		output := string(raw)
		if !strings.HasPrefix(output, "elastic-agent-") {
			return res.WithError(fmt.Errorf("unexpected result when inspecting Elastic Agent container %s", output))
		}

		vcsRef := filepath.Base(strings.Trim(output, "\n"))
		knownRefs[params.Agent.Spec.Version] = vcsRef
		params.AgentVCSRef = vcsRef

		params.Logger().V(1).Info("Elastic Agent container inspection pod succeeded", "vcs-ref", vcsRef)
		err = params.Client.Delete(context.Background(), &pod)
		return res.WithError(err)
	case corev1.PodFailed:
		err := fmt.Errorf("cannot inspect Elastic Agent container, pod failed: %v", pod.Status)
		return res.WithError(err)
	default:
		params.Logger().V(1).Info("Waiting on Elastic Agent container inspection", "phase", pod.Status.Phase)
		return res.WithResult(requeueResult)
	}
}
