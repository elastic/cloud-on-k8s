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
// is computed from what changed: with spec.nodeSets declared as a keyed list, that is exactly
// nodeSets[name=...].count and nodeSets[name=...].resources.
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
		// reconciled is derived from current without reordering, so indexes line up. The test
		// operation below turns any violation of that into a failed patch rather than a bad write.
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
			ops = append(ops, jsonPatchOp{Op: "add", Path: fmt.Sprintf("/spec/nodeSets/%d/resources", i), Value: next.Resources})
		}
	}

	if len(ops) == 0 {
		return nil
	}

	patch, err := json.Marshal(ops)
	if err != nil {
		return err
	}
	return c.Patch(ctx, reconciled, client.RawPatch(types.JSONPatchType, patch))
}
