// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"encoding/json"
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// jsonPatchOp is a single RFC 6902 operation.
type jsonPatchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

// patchAutoscaledNodeSets persists the autoscaler's recommendations with a JSON patch carrying
// nothing but the count and resources of the NodeSets whose values actually changed. It issues no
// request when nothing changed.
//
// This replaces a full-object Update. Under Server-Side Apply an Update makes the requester the
// owner of every field it adds, and marshalling an Elasticsearch emits all fields that lack
// omitempty as empty objects, so the autoscaler ended up owning spec.auth, spec.http,
// spec.monitoring, spec.transport, spec.remoteClusterServer and spec.updateStrategy on top of what
// it meant to manage. A JSON patch is still an Update as far as field management goes, so ownership
// is computed from what changed: with spec.nodeSets declared as a keyed list, the autoscaler
// claims nodeSets[name=...].count and, within resources, only the individual leaves it wrote
// (requests.cpu, limits.cpu, requests.memory, limits.memory, storage).
//
// A Server-Side Apply would scope ownership just as well but is the wrong tool here, because Apply
// prunes: a manager that stops sending a field it owns deletes it. When a policy is removed from
// the ElasticsearchAutoscaler, or its roles change so a NodeSet no longer matches, the next apply
// would omit that NodeSet and drop its count, which decodes to zero and scales the tier down to
// nothing. A patch has no such semantics.
//
// Each write is guarded by a test operation on the NodeSet name, so a concurrent reorder or rename
// makes the patch fail rather than write the recommendation to the wrong NodeSet.
func patchAutoscaledNodeSets(ctx context.Context, c k8s.Client, current, reconciled *esv1.Elasticsearch) error {
	var ops []jsonPatchOp

	for i := range reconciled.Spec.NodeSets {
		next := reconciled.Spec.NodeSets[i]
		// reconciled is derived from current without reordering, so indexes line up; if reconciled
		// has fewer entries than current the remainder are skipped (unchanged), and the test
		// operation below rejects any write that would land on the wrong NodeSet in the stored object.
		if i >= len(current.Spec.NodeSets) {
			return fmt.Errorf("unexpected nodeSet %s at index %d, not present in the current Elasticsearch", next.Name, i)
		}
		previous := current.Spec.NodeSets[i]

		countChanged := previous.Count != next.Count
		resourcesChanged := !apiequality.Semantic.DeepEqual(previous.Resources, next.Resources)
		if !countChanged && !resourcesChanged {
			continue
		}

		ops = append(ops, jsonPatchOp{Op: "test", Path: fmt.Sprintf("/spec/nodeSets/%d/name", i), Value: next.Name})
		if countChanged {
			// "add" rather than "replace": it upserts, and count may be absent from the stored
			// object when the chart does not template it.
			ops = append(ops, jsonPatchOp{Op: "add", Path: fmt.Sprintf("/spec/nodeSets/%d/count", i), Value: next.Count})
		}
		if resourcesChanged {
			ops = append(ops, resourcesLeafOps(fmt.Sprintf("/spec/nodeSets/%d/resources", i), previous.Resources, next.Resources)...)
		}
	}

	if len(ops) == 0 {
		return nil
	}

	ops = append([]jsonPatchOp{{Op: "test", Path: "/metadata/resourceVersion", Value: current.ResourceVersion}}, ops...)

	patch, err := json.Marshal(ops)
	if err != nil {
		return err
	}
	return c.Patch(ctx, reconciled, client.RawPatch(types.JSONPatchType, patch))
}

// resourcesLeafOps returns RFC 6902 ops for the resource fields that changed between prev and next.
// CPU and memory are grouped as ownership units across requests and limits: when a parent path
// (/requests or /limits) does not yet exist it must be created in a single "add" op carrying all
// managed fields, so both sides of a resource unit are resolved together before any op is emitted.
// When /resources itself is absent a single object-level "add" is used, because JSON Patch rejects
// ops that target a non-existent path.
func resourcesLeafOps(basePath string, prev, next esv1.NodeSetResources) []jsonPatchOp {
	resourcesExists := !prev.IsEmpty() || prev.Storage != nil
	if !resourcesExists {
		if obj := buildResourcesMap(next); len(obj) > 0 {
			return []jsonPatchOp{{Op: "add", Path: basePath, Value: obj}}
		}
		return nil
	}

	cpuManaged := !apiequality.Semantic.DeepEqual(prev.Requests.CPU, next.Requests.CPU) ||
		!apiequality.Semantic.DeepEqual(prev.Limits.CPU, next.Limits.CPU)
	memManaged := !apiequality.Semantic.DeepEqual(prev.Requests.Memory, next.Requests.Memory) ||
		!apiequality.Semantic.DeepEqual(prev.Limits.Memory, next.Limits.Memory)

	var ops []jsonPatchOp
	ops = append(ops, managedAllocationLeafOps(basePath+"/requests", prev.Requests, next.Requests, cpuManaged, memManaged)...)
	ops = append(ops, managedAllocationLeafOps(basePath+"/limits", prev.Limits, next.Limits, cpuManaged, memManaged)...)

	if !apiequality.Semantic.DeepEqual(prev.Storage, next.Storage) {
		ops = append(ops, jsonPatchOp{Op: "add", Path: basePath + "/storage", Value: next.Storage})
	}
	return ops
}

// managedAllocationLeafOps emits ops for a single allocation group (requests or limits).
// parentExists checks both CPU and Memory from prev so that a group containing only one managed
// resource does not try to recreate a parent path that already exists for the other resource.
func managedAllocationLeafOps(
	parentPath string,
	prev, next commonv1.ResourceAllocations,
	cpuManaged, memManaged bool,
) []jsonPatchOp {
	parentExists := prev.CPU != nil || prev.Memory != nil
	if !parentExists {
		obj := map[string]any{}
		if cpuManaged && next.CPU != nil {
			obj["cpu"] = next.CPU
		}
		if memManaged && next.Memory != nil {
			obj["memory"] = next.Memory
		}
		if len(obj) == 0 {
			return nil
		}
		return []jsonPatchOp{{Op: "add", Path: parentPath, Value: obj}}
	}
	var ops []jsonPatchOp
	if cpuManaged && !apiequality.Semantic.DeepEqual(prev.CPU, next.CPU) {
		ops = append(ops, jsonPatchOp{Op: "add", Path: parentPath + "/cpu", Value: next.CPU})
	}
	if memManaged && !apiequality.Semantic.DeepEqual(prev.Memory, next.Memory) {
		ops = append(ops, jsonPatchOp{Op: "add", Path: parentPath + "/memory", Value: next.Memory})
	}
	return ops
}

// buildResourcesMap returns a map of the non-nil fields in next, used to create /resources
// as a single op when the path does not yet exist in the stored object.
func buildResourcesMap(next esv1.NodeSetResources) map[string]any {
	obj := map[string]any{}
	if req := buildAllocationMap(next.Requests); len(req) > 0 {
		obj["requests"] = req
	}
	if lim := buildAllocationMap(next.Limits); len(lim) > 0 {
		obj["limits"] = lim
	}
	if next.Storage != nil {
		obj["storage"] = next.Storage
	}
	return obj
}

func buildAllocationMap(a commonv1.ResourceAllocations) map[string]any {
	m := map[string]any{}
	if a.CPU != nil {
		m["cpu"] = a.CPU
	}
	if a.Memory != nil {
		m["memory"] = a.Memory
	}
	return m
}
