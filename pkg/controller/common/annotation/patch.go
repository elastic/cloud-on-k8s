// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package annotation

import (
	"context"
	"encoding/json"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// PatchAnnotations changes only metadata.annotations on obj: entries in set are written, keys in
// remove are deleted, and every other field is left untouched. It is a no-op, issuing no request at
// all, when the object already has the desired annotations.
//
// This exists so that annotation-only writes stop going through a full-object Update. Under
// Server-Side Apply the API server records the requester as the owner of every field its request
// adds, and marshalling a resource emits all of its fields that lack omitempty, as empty objects
// when unset. Writing a single annotation with Update therefore also claims spec fields the
// operator never meant to manage. Where the claimed field is atomic, such as Elasticsearch's
// spec.nodeSets, the claim covers the whole subtree, which is what makes a subsequent
// `helm upgrade` fail with "conflict with elastic-operator ... .spec.nodeSets".
//
// resourceVersion is embedded in the patch so this keeps the optimistic concurrency guarantee that
// Update provided: a caller holding a stale object gets a conflict instead of silently clobbering a
// concurrent change.
func PatchAnnotations(ctx context.Context, c k8s.Client, obj client.Object, set map[string]string, remove ...string) error {
	current := obj.GetAnnotations()

	// Only send what actually differs. A JSON merge patch expresses removal as an explicit null.
	changes := make(map[string]any, len(set)+len(remove))
	for key, value := range set {
		if currentValue, exists := current[key]; !exists || currentValue != value {
			changes[key] = value
		}
	}
	for _, key := range remove {
		if _, exists := current[key]; exists {
			changes[key] = nil
		}
	}
	if len(changes) == 0 {
		return nil
	}

	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"annotations":     changes,
			"resourceVersion": obj.GetResourceVersion(),
		},
	})
	if err != nil {
		return err
	}

	// Patch writes the server's response back into obj, so the caller's copy stays in sync.
	return c.Patch(ctx, obj, client.RawPatch(types.MergePatchType, patch))
}
