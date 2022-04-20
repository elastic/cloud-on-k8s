// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"context"
	"fmt"
	"strings"

	"go.elastic.co/apm"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// ResourceNotAvailable implements the error interface and can be used to raise cases where not all compute or
// storage resources have been detected.
type ResourceNotAvailable struct {
	nodeSet string
	reasons []string
}

func (r ResourceNotAvailable) Error() string {
	return fmt.Sprintf("cannot compute resources for NodeSet \"%s\": %s", r.nodeSet, strings.Join(r.reasons, ", "))
}

func (r ResourceNotAvailable) WithReason(reason string) ResourceNotAvailable {
	if reason == "" {
		return r
	}
	return ResourceNotAvailable{
		nodeSet: r.nodeSet,
		reasons: append(r.reasons, reason),
	}
}

type nodeResources []nodeResource

func (n nodeResources) requeue() bool {
	for _, nodeResource := range n {
		if nodeResource.requeue {
			return true
		}
	}
	return false
}

type nodeResource struct {
	nodeName             string
	cpu, memory, storage int64
	requeue              bool
}

type nodeSetResourcesBuilder struct {
	nodeSet string
	cpu     int64
	memory  int64
	reasons []string
}

func (n nodeSetResourcesBuilder) addReason(reason string) nodeSetResourcesBuilder {
	n.reasons = append(n.reasons, reason)
	return n
}

func (n nodeSetResourcesBuilder) toError() error {
	if len(n.reasons) == 0 {
		return nil
	}
	return &ResourceNotAvailable{nodeSet: n.nodeSet, reasons: n.reasons}
}

const (
	envPodName             = "${" + settings.EnvPodName + "}"
	envNamespace           = "${" + settings.EnvNamespace + "}"
	envHeadlessServiceName = "${" + settings.HeadlessServiceName + "}"
)

// ToDesiredNodes returns the desired nodes, as expected by the desired nodes API, from an expected resources list.
func (l ResourcesList) ToDesiredNodes(
	ctx context.Context,
	k8sClient k8s.Client,
	version string,
) ([]client.DesiredNode, bool, error) {
	span, ctx := apm.StartSpan(ctx, "compute_desired_nodes", tracing.SpanTypeApp)
	defer span.End()
	// Pre-compute the size of the desired nodes slice.
	var nodesCount int32
	for _, resources := range l {
		nodesCount += sset.GetReplicas(resources.StatefulSet)
	}
	requeue := false
	desiredNodes := make([]client.DesiredNode, 0, nodesCount)
	for _, resources := range l {
		sts := resources.StatefulSet
		esContainer := getElasticsearchContainer(sts.Spec.Template.Spec.Containers)
		if esContainer == nil {
			return nil, false, fmt.Errorf("cannot find Elasticsearch container in StatefulSet %s/%s", sts.Namespace, sts.Name)
		}

		nodeResources, err := nodeSetResourcesBuilder{nodeSet: resources.NodeSet}.
			withProcessors(esContainer.Resources).
			withMemory(esContainer.Resources).
			withStorage(ctx, k8sClient, sts, resources.Config, esContainer)
		if err != nil {
			return nil, false, err
		}

		requeue = requeue || nodeResources.requeue()

		for _, nodeResource := range nodeResources {
			// Replace variable in the configuration
			knownVariablesReplacer := strings.NewReplacer(
				envPodName, nodeResource.nodeName,
				envNamespace, sts.Namespace,
				envHeadlessServiceName, resources.HeadlessService.Name,
			)
			var settings map[string]interface{}
			if err := resources.Config.CanonicalConfig.Unpack(&settings); err != nil {
				return nil, false, err
			}
			visit(nil, settings, func(s string) string {
				return knownVariablesReplacer.Replace(s)
			})

			node := client.DesiredNode{
				NodeVersion: version,
				Processors:  int(nodeResource.cpu), // we assume the number of CPU does not overflow an int64
				Memory:      fmt.Sprintf("%db", nodeResource.memory),
				Storage:     fmt.Sprintf("%db", nodeResource.storage),
				Settings:    settings,
			}
			desiredNodes = append(desiredNodes, node)
		}
	}

	return desiredNodes, requeue, nil
}

// visit recursively visits a map holding a tree structure and apply a function to nodes that hold a string.
func visit(keys []string, m map[string]interface{}, apply func(string) string) {
	for k, v := range m {
		if childMap, isMap := v.(map[string]interface{}); isMap {
			visit(append(keys, k), childMap, apply)
		}
		if value, isString := v.(string); isString {
			m[k] = apply(value)
		}
	}
}

// getElasticsearchContainer returns the Elasticsearch container, or nil if not found.
func getElasticsearchContainer(containers []corev1.Container) *corev1.Container {
	for _, c := range containers {
		if c.Name == esv1.ElasticsearchContainerName {
			return &c
		}
	}
	return nil
}

// withProcessors computes the available CPU resource for the Elasticsearch container.
// It uses the limit if provided, otherwise fallback to the requirement.
// It returns nil if the neither the request nor a limit is set.
func (n nodeSetResourcesBuilder) withProcessors(resources corev1.ResourceRequirements) nodeSetResourcesBuilder {
	// Try to get the limit
	limit, hasLimit := resources.Limits[corev1.ResourceCPU]
	if hasLimit {
		if limit.IsZero() {
			return n.addReason("CPU limit is set but value is 0")
		}
		n.cpu = limit.Value()
		return n
	}
	// Try to get the request
	request, hasRequest := resources.Requests[corev1.ResourceCPU]
	if hasRequest {
		if request.IsZero() {
			return n.addReason("CPU request is set but value is 0")
		}
		n.cpu = request.Value()
		return n
	}
	// Neither the limit nor the request is set
	return n.addReason("no CPU request or limit set")
}

// withMemory computes the available memory resource.
// It returns nil if the limit and the request do not have the same value.
func (n nodeSetResourcesBuilder) withMemory(resources corev1.ResourceRequirements) nodeSetResourcesBuilder {
	limit, hasLimit := resources.Limits[corev1.ResourceMemory]
	request, hasRequest := resources.Requests[corev1.ResourceMemory]
	switch {
	case !hasLimit:
		// Having a memory limit is mandatory to guess the allocated memory.
		return n.addReason("memory limit is not set")
	case hasLimit && hasRequest && !limit.Equal(request):
		// If request is set it must have the same value as the limit.
		return n.addReason("memory request and limit do not have the same value")
	}
	if limit.IsZero() {
		return n.addReason("Memory limit is set but value is 0")
	}
	n.memory = limit.Value()
	return n
}

// getStorage attempts to detect the storage capacity of the Elasticsearch nodes.
// 1. Attempt to detect path settings, an error is raised if multiple data paths are set.
// 2. Detect the data volume name. If none can be detected an error is raised.
// 3. Lookup for the corresponding volume claim.
// 4. For each Pod in the StatefulSet we attempt to read the capacity from the PVC status or from the Spec
//    if there is no status yet.
func (n nodeSetResourcesBuilder) withStorage(
	ctx context.Context,
	k8sClient k8s.Client,
	statefulSet appsv1.StatefulSet,
	config settings.CanonicalConfig,
	esContainer *corev1.Container,
) (nodeResources, error) {
	var p pathSetting
	if err := config.CanonicalConfig.Unpack(&p); err != nil {
		return nil, err
	}
	pathData, ok := p.PathData.(string)
	if !ok {
		return nil, n.addReason("Elasticsearch path.data must be a string, multiple paths is not supported").toError()
	}

	var volumeName string
	for _, mount := range esContainer.VolumeMounts {
		if mount.MountPath == pathData {
			volumeName = mount.Name
			continue
		}
	}
	if len(volumeName) == 0 {
		return nil, n.addReason(fmt.Sprintf("Elasticsearch path.data %s must mounted by a volume", pathData)).toError()
	}

	var esDataVolumeClaim *corev1.PersistentVolumeClaim
	for _, pvc := range statefulSet.Spec.VolumeClaimTemplates {
		if pvc.Name == volumeName {
			pvc := pvc // return a pointer on a copy
			esDataVolumeClaim = &pvc
			continue
		}
	}

	if esDataVolumeClaim == nil {
		return nil, n.addReason(fmt.Sprintf("Volume claim with name \"%s\" not found in Spec.VolumeClaimTemplates ", pathData)).toError()
	}

	claimedStorage := getClaimedStorage(*esDataVolumeClaim)
	if claimedStorage == nil {
		return nil, n.addReason(fmt.Sprintf("No storage request in claim \"%s\"", esDataVolumeClaim.Name)).toError()
	}

	// Stop here if there is at least one reason to not compute the desired state.
	if err := n.toError(); err != nil {
		return nil, err
	}

	nodeResources := make([]nodeResource, sset.GetReplicas(statefulSet))
	for i, podName := range sset.PodNames(statefulSet) {
		nodeResources[i].nodeName = podName
		nodeResources[i].cpu = n.cpu
		nodeResources[i].memory = n.memory
		pvcName := fmt.Sprintf("%s-%s", esDataVolumeClaim.Name, podName)
		pvc := corev1.PersistentVolumeClaim{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: statefulSet.Namespace, Name: pvcName}, &pvc); err != nil {
			if apierrors.IsNotFound(err) {
				// PVC does not exist (yet)
				nodeResources[i].requeue = true
				nodeResources[i].storage = *claimedStorage
				continue
			}
			return nil, err
		}
		// We first attempt to read the PVC status
		if storageInStatus, exists := pvc.Status.Capacity[corev1.ResourceStorage]; exists {
			nodeResources[i].storage = storageInStatus.Value()
			continue
		}
		// If there is no storage value in the status use the value in the spec
		nodeResources[i].requeue = true
		if storageInSpec, exists := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; exists {
			nodeResources[i].storage = storageInSpec.Value()
		} else {
			// fall back to claimed storage
			nodeResources[i].storage = *claimedStorage
		}
	}

	return nodeResources, nil
}

func getClaimedStorage(claim corev1.PersistentVolumeClaim) *int64 {
	if storage, exists := claim.Spec.Resources.Requests[corev1.ResourceStorage]; exists {
		return pointer.Int64Ptr(storage.Value())
	}
	return nil
}

// pathSetting captures secrets settings in the Kibana configuration that we want to reuse.
type pathSetting struct {
	PathData interface{} `config:"path.data"`
}
