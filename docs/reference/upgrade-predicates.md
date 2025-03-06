---
navigation_title: ECK upgrade predicates
applies_to:
  deployment:
    eck: all
---

# ECK rolling upgrade predicates

## Advanced control during rolling upgrades [k8s-advanced-upgrade-control]

(work in progress - reference content)

The rules (otherwise known as predicates) that the ECK operator follows during an Elasticsearch upgrade can be selectively disabled for certain scenarios where the ECK operator will not proceed with an Elasticsearch cluster upgrade because it deems it to be "unsafe".

::::{warning}
Selectively disabling the predicates listed in this section are extremely risky, and carry a high chance of either data loss, or causing a cluster to become completely unavailable. Use them only if you are sure that you are not causing permanent damage to an Elasticsearch cluster. These predicates might change in the future. We will be adding, removing, and renaming these over time, so be careful in adding these to any automation.  Also, make sure you remove them after use. `kublectl annotate elasticsearch.elasticsearch.k8s.elastic.co/elasticsearch-sample eck.k8s.elastic.co/disable-upgrade-predicates-`
::::


* The following named predicates control the upgrade process

    * data_tier_with_higher_priority_must_be_upgraded_first

        Upgrade the frozen tier first, then the cold tier, then the warm tier, and the hot tier last. This ensures ILM can continue to move data through the tiers during the upgrade.

    * do_not_restart_healthy_node_if_MaxUnavailable_reached

        If `maxUnavailable` is reached, only allow unhealthy Pods to be deleted.

    * skip_already_terminating_pods

        Do not attempt to restart pods that are already in the process of being terminated.

    * only_restart_healthy_node_if_green_or_yellow

        Only restart healthy Elasticsearch nodes if the health of the cluster is either green or yellow, never red.

    * if_yellow_only_restart_upgrading_nodes_with_unassigned_replicas

        During a rolling upgrade, primary shards assigned to a node running a new version cannot have their replicas assigned to a node with the old version. Therefore we must allow some Pods to be restarted even if cluster health is yellow so the replicas can be assigned.

    * require_started_replica

        If a cluster is yellow, allow deleting a node, but only if they do not contain the only replica of a shard since it would make the cluster go red.

    * one_master_at_a_time

        Only allow a single master to be upgraded at a time.

    * do_not_delete_last_master_if_all_master_ineligible_nodes_are_not_upgraded

        Force an upgrade of all the master-ineligible nodes before upgrading the last master-eligible node.

    * do_not_delete_pods_with_same_shards

        Do not allow two pods containing the same shard to be deleted at the same time.

    * do_not_delete_all_members_of_a_tier

        Do not delete all nodes that share the same node roles at once. This ensures that there is always availability for each configured tier of nodes during a rolling upgrade.


Any of these predicates can be disabled by adding an annotation with the key of `eck.k8s.elastic.co/disable-upgrade-predicates` to the Elasticsearch metadata, specifically naming the predicate that is needing to be disabled.  Also, all predicates can be disabled by replacing the name of any predicatae with "*".

* Example use case

Assume a given Elasticsearch cluster is a "red" state because of an un-allocatable shard setting that was applied to the cluster:

```json
{
	"settings": {
		"index.routing.allocation.include._id": "does not exist"
	}
}
```

This cluster would never be allowed to be upgraded with the standard set of upgrade predicates in place, as the cluster is in a "red" state, and the named predicate `only_restart_healthy_node_if_green_or_yellow` prevents the upgrade.

If the following annotation was added to the cluster specification, and the version was increased from 7.15.2 → 7.15.3

```yaml
apiVersion: elasticsearch.k8s.elastic.co/v1
kind: Elasticsearch
metadata:
  name: testing
  annotations:
    eck.k8s.elastic.co/disable-upgrade-predicates: "only_restart_healthy_node_if_green_or_yellow"
    # Also note that eck.k8s.elastic.co/disable-upgrade-predicates: "*" would work as well, but is much less selective.
spec:
  version: 7.15.3 # previously set to 7.15.2, for example
```

The ECK operator would allow this upgrade to proceed, even though the cluster was in a "red" state during this upgrade process.


