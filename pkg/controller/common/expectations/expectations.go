// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package expectations

import (
	"fmt"
	"strings"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

/*

# Expectations FAQ

## What are expectations?
Expectations are in-memory data structures that we use to register an action we performed during a reconciliation.
Example: a StatefulSet was updated, a Pod was deleted.

## Why do we need them?
Our Kubernetes client caches all resources locally, and relies on apiserver watches to maintain the cache up-to-date.
The cache is never invalidated. For example, it is possible to:

1. Delete a Pod.
2. List all Pods: the deleted Pod is still there in the cache (and not marked for termination). It's impossible
to know it was actually deleted before.
3. List all Pods again later: the Pod is correctly deleted.

These steps may correspond to different reconciliation attempts. Step 2 can be particularly dangerous when dealing
with external systems such as Elasticsearch. What if we update Elasticsearch orchestration settings based on an
out-of-date number of Pods?

## What are we tracking exactly? When?

* Updates on StatefulSets specification, that we track through the StatefulSet Generation attribute. They are updated
every time we do an update on StatefulSets, changing the spec or number of replicas.
* Pods deletions, that we track using UID of deleted Pods. They are updated every time we manually delete a Pod during
a rolling upgrade. They are not updated during downscales: the updated StatefulSets replicas is tracked through the
StatefulSets generation expectations.

## Give me some concrete examples why this is useful!

Things that could happen without this mechanism in place:
- create more than one master node at a time
- delete more than one master node at a time
- outgrow the changeBudget during upscale and downscales
- clear shards allocation excludes for a node that is not removed yet
- update zen1/zen2 minimum_master_nodes/initial_master_nodes based on the wrong number of nodes
- update zen1/zen2 minimum_master_nodes/initial_master_nodes based on the wrong nodes specification (ignoring master->data upgrades)
- clear voting_config_exclusions while a Pod has not finished its restart yet (or maybe just started)

## What if the operator restarts?

All in-memory expectations are lost if the operator restarts. This is fine, because the operator re-populates its cache
with the current resources in the apiserver. These resources do take into account any create/update/delete operation
that was performed before the operator restarted.

## Don't we need that for... basically everything?

No. In most situations, it's totally fine to rely on Kubernetes optimistic locking:
* if we create a resource that already exists, the operation fails
* if we update an out-of-date resource, the operation fails
* if we delete a resource that does not exist, the operation fails

The only cases where we need it are (so far):
- when interacting with external systems own orchestration mechanism (Elasticsearch zen1/zen2)
- when trying to control how many creations/deletions/upgrades happen in parallel

## Where does it come from?

Kubernetes Deployment controller relies on expectations to control pods creation and deletion:
https://github.com/kubernetes/kubernetes/blob/245189b8a198e9e29494b2d992dc05bd7164c973/pkg/controller/controller_utils.go#L115
Our expectations mechanism is inspired by it.
A major difference is the Deployments one is directly plugged to watches events to track creation and deletion.
Instead, we just inspect resources in the cache when we need to; which makes it a bit simpler to understand, but
technically less efficient.

*/

// Expectations stores expectations for a single cluster. It is not thread-safe.
type Expectations struct {
	*ExpectedStatefulSetUpdates
	*ExpectedPodDeletions
}

// NewExpectations returns an initialized Expectations.
func NewExpectations(client k8s.Client) *Expectations {
	return &Expectations{
		ExpectedStatefulSetUpdates: NewExpectedStatefulSetUpdates(client),
		ExpectedPodDeletions:       NewExpectedPodDeletions(client),
	}
}

// Satisfied returns true if both deletions and generations are expected.
func (e *Expectations) Satisfied() (bool, string, error) {
	pendingPodDeletions, err := e.PendingPodDeletions()
	if err != nil {
		return false, "", err
	}
	if len(pendingPodDeletions) > 0 {
		return false, fmt.Sprintf("Expecting deletion for Pods: %s", strings.Join(pendingPodDeletions, ",")), nil
	}
	pendingGenerations, err := e.PendingGenerations()
	if err != nil {
		return false, "", err
	}
	if len(pendingGenerations) > 0 {
		return false, fmt.Sprintf("StatefulSets not reconciled yet: %s", strings.Join(pendingGenerations, ",")), nil
	}
	return true, "", nil
}
