// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package reconcile

import (
	"reflect"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/shutdown"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
)

type StatusReporter struct {
	esv1.Conditions
	*UpscaleReporter
	*DownscaleReporter
	*UpgradeReporter
}

// MergeStatusReportingWith creates a new ElasticsearchStatus merging the reported status and an existing ElasticsearchStatus.
func (s *StatusReporter) MergeStatusReportingWith(otherStatus esv1.ElasticsearchStatus) esv1.ElasticsearchStatus {
	mergedStatus := otherStatus.DeepCopy()
	mergedStatus.UpgradeOperation = s.UpgradeReporter.Merge(otherStatus.UpgradeOperation)
	mergedStatus.UpscaleOperation = s.UpscaleReporter.Merge(otherStatus.UpscaleOperation)
	mergedStatus.DownscaleOperation = s.DownscaleReporter.Merge(otherStatus.DownscaleOperation)

	// Merge conditions
	for _, condition := range s.Conditions {
		mergedStatus.Conditions = mergedStatus.Conditions.MergeWith(condition)
	}

	return *mergedStatus
}

// ReportCondition records a condition to be reported in the status.
// Any existing condition with the same Type is overridden.
func (s *StatusReporter) ReportCondition(
	conditionType esv1.ConditionType,
	status corev1.ConditionStatus,
	message string) {
	s.Conditions = s.Conditions.MergeWith(esv1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Message:            message,
	})
}

// -- Upscale status

type UpscaleReporter struct {
	// Expected nodes to be upscaled
	nodes map[string]esv1.NewNode
}

// RecordNewNodes records pending node creations.
func (u *UpscaleReporter) RecordNewNodes(nodes []string) {
	if u == nil {
		return
	}
	if u.nodes == nil {
		u.nodes = make(map[string]esv1.NewNode, len(nodes))
	}
	for _, node := range nodes {
		newNode := u.nodes[node]
		newNode.Name = node
		newNode.Status = esv1.NewNodePending
		u.nodes[node] = newNode
	}
}

// UpdateNodesStatuses updates the status and the message fields for a set of nodes belonging to the same StatefulSet.
func (u *UpscaleReporter) UpdateNodesStatuses(status esv1.NewNodeStatus, statefulSetName, message string, minOrdinal, maxOrdinal int32) {
	if u == nil {
		return
	}
	if u.nodes == nil {
		u.nodes = make(map[string]esv1.NewNode)
	}
	for ord := minOrdinal - 1; ord < maxOrdinal; ord++ {
		podName := sset.PodName(statefulSetName, ord)
		newNode := u.nodes[podName]
		newNode.Status = status
		newNode.Message = pointer.String(message)
		u.nodes[podName] = newNode
	}
}

// HasPendingNewNodes returns true if at least one pending node creation has been reported.
func (u *UpscaleReporter) HasPendingNewNodes() bool {
	if u == nil {
		return false
	}
	return len(u.nodes) > 0
}

// Merge creates a new upscale status using the reported upscale status and an existing upscale status.
func (u *UpscaleReporter) Merge(other esv1.UpscaleOperation) esv1.UpscaleOperation {
	upscaleOperation := other.DeepCopy()
	if u == nil {
		return *upscaleOperation
	}
	nodes := make([]esv1.NewNode, 0, len(u.nodes))
	for name, node := range u.nodes {
		nodes = append(nodes, esv1.NewNode{
			Name:    name,
			Status:  node.Status,
			Message: node.Message,
		})
	}
	// Sort for stable comparison
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})
	if (u.nodes != nil && !reflect.DeepEqual(nodes, other.Nodes)) || upscaleOperation.Nodes == nil {
		upscaleOperation.Nodes = nodes
		upscaleOperation.LastUpdatedTime = metav1.Now()
	}
	return *upscaleOperation
}

// -- Upgrade status

type UpgradeReporter struct {
	// Expected nodes to be upgraded, key is node name
	nodes map[string]esv1.UpgradedNode
}

// RecordNodesToBeUpgraded records in the status a list of nodes that should be upgraded.
func (u *UpgradeReporter) RecordNodesToBeUpgraded(nodes []string) {
	u.RecordNodesToBeUpgradedWithMessage(nodes, "")
}

func (u *UpgradeReporter) recordNodesUpgrade(nodes []string, status string, message string) {
	if u == nil {
		return
	}
	if u.nodes == nil {
		u.nodes = make(map[string]esv1.UpgradedNode, len(nodes))
	}
	for _, node := range nodes {
		upgradedNode := u.nodes[node]
		upgradedNode.Name = node
		upgradedNode.Status = status
		if len(message) > 0 {
			upgradedNode.Message = pointer.String(message)
		}
		u.nodes[node] = upgradedNode
	}
}

// RecordNodesToBeUpgradedWithMessage records in the status a list of nodes that should be upgraded
// with an additional message to give more information when relevant.
func (u *UpgradeReporter) RecordNodesToBeUpgradedWithMessage(nodes []string, message string) {
	u.recordNodesUpgrade(nodes, "PENDING", message)
}

// RecordDeletedNode records a node being deleted for upgrade.
func (u *UpgradeReporter) RecordDeletedNode(node, message string) {
	u.recordNodesUpgrade([]string{node}, "DELETED", message)
}

// RecordPredicatesResult records predicates results for a set of nodes.
func (u *UpgradeReporter) RecordPredicatesResult(predicatesResult map[string]string) {
	if u == nil {
		return
	}
	if u.nodes == nil {
		u.nodes = make(map[string]esv1.UpgradedNode, len(predicatesResult))
	}
	for node, predicate := range predicatesResult {
		upgradedNode := u.nodes[node]
		upgradedNode.Name = node
		upgradedNode.Predicate = pointer.String(predicate)
		upgradedNode.Message = pointer.String("Cannot restart node because of failed predicate")
		u.nodes[node] = upgradedNode
	}
}

// Merge creates a new upgrade status using the reported upgrade status and an existing upgrade status.
func (u *UpgradeReporter) Merge(other esv1.UpgradeOperation) esv1.UpgradeOperation {
	upgradeOperation := other.DeepCopy()
	if u == nil {
		return *upgradeOperation
	}
	nodes := make([]esv1.UpgradedNode, 0, len(u.nodes))
	for _, node := range u.nodes {
		nodes = append(nodes, esv1.UpgradedNode{
			Name:      node.Name,
			Predicate: node.Predicate,
			Message:   node.Message,
			Status:    node.Status,
		})
	}
	// Sort for stable comparison
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})
	if (u.nodes != nil && !reflect.DeepEqual(nodes, other.Nodes)) || upgradeOperation.Nodes == nil {
		upgradeOperation.Nodes = nodes
		upgradeOperation.LastUpdatedTime = metav1.Now()
	}
	return *upgradeOperation
}

// -- Downscale status

type DownscaleReporter struct {
	// Expected nodes to be downscaled, key is node name
	nodes   map[string]esv1.DownscaledNode
	stalled *bool
}

// RecordNodesToBeRemoved records nodes expected to be eventually removed from the cluster.
func (d *DownscaleReporter) RecordNodesToBeRemoved(nodes []string) {
	if d == nil {
		return
	}
	if d.nodes == nil {
		d.nodes = make(map[string]esv1.DownscaledNode, len(nodes))
	}
	for _, node := range nodes {
		d.nodes[node] = esv1.DownscaledNode{
			Name: node,
			// We set an initial value to let the caller know that this node should be eventually deleted.
			// This should be overridden by the downscale algorithm.
			ShutdownStatus: "NOT_STARTED",
		}
	}
}

// Merge creates a new downscale status using the reported downscale status and an existing downscale status.
func (d *DownscaleReporter) Merge(other esv1.DownscaleOperation) esv1.DownscaleOperation {
	downscaleOperation := other.DeepCopy()
	if d == nil {
		return other
	}
	nodes := make([]esv1.DownscaledNode, 0, len(d.nodes))
	for _, node := range d.nodes {
		nodes = append(nodes, esv1.DownscaledNode{
			Name:           node.Name,
			ShutdownStatus: node.ShutdownStatus,
			Explanation:    node.Explanation,
		})
	}
	// Sort for stable comparison
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})
	if (d.nodes != nil && !reflect.DeepEqual(nodes, other.Nodes)) || downscaleOperation.Nodes == nil {
		downscaleOperation.Nodes = nodes
		downscaleOperation.LastUpdatedTime = metav1.Now()
	}

	if !reflect.DeepEqual(d.stalled, other.Stalled) {
		downscaleOperation.Stalled = d.stalled
		downscaleOperation.LastUpdatedTime = metav1.Now()
	}

	return *downscaleOperation
}

func (d *DownscaleReporter) OnShutdownStatus(
	podName string,
	nodeShutdownStatus shutdown.NodeShutdownStatus,
) {
	if d == nil {
		return
	}
	if d.nodes == nil {
		d.nodes = make(map[string]esv1.DownscaledNode)
	}
	node := d.nodes[podName]
	node.Name = podName
	node.ShutdownStatus = string(nodeShutdownStatus.Status)
	if len(nodeShutdownStatus.Explanation) > 0 {
		node.Explanation = pointer.StringPtr(nodeShutdownStatus.Explanation)
	}
	d.nodes[podName] = node
	if nodeShutdownStatus.Status == esclient.ShutdownStalled {
		d.stalled = pointer.Bool(true)
	}
}

func (d *DownscaleReporter) OnReconcileShutdowns(leavingNodes []string) {
	if d == nil {
		return
	}
	if d.nodes == nil {
		d.nodes = make(map[string]esv1.DownscaledNode)
	}
	// Update InProgress condition and DownscaleOperation
	for _, nodeName := range leavingNodes {
		node := d.nodes[nodeName]
		node.Name = nodeName
		node.ShutdownStatus = string(esclient.ShutdownInProgress)
		d.nodes[nodeName] = node
	}
}
