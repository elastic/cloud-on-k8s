// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

// ElasticsearchHealth is the health of the cluster as returned by the health API.
type ElasticsearchHealth string

// Possible traffic light states Elasticsearch health can have.
const (
	ElasticsearchRedHealth     ElasticsearchHealth = "red"
	ElasticsearchYellowHealth  ElasticsearchHealth = "yellow"
	ElasticsearchGreenHealth   ElasticsearchHealth = "green"
	ElasticsearchUnknownHealth ElasticsearchHealth = "unknown"
)

var elasticsearchHealthOrder = map[ElasticsearchHealth]int{
	ElasticsearchRedHealth:    1,
	ElasticsearchYellowHealth: 2,
	ElasticsearchGreenHealth:  3,
}

// Less for ElasticsearchHealth means green > yellow > red
func (h ElasticsearchHealth) Less(other ElasticsearchHealth) bool {
	l := elasticsearchHealthOrder[h]
	r := elasticsearchHealthOrder[other]
	// 0 is not found/unknown and less is not defined for that
	return l != 0 && r != 0 && l < r
}

// ElasticsearchOrchestrationPhase is the phase Elasticsearch is in from the controller point of view.
type ElasticsearchOrchestrationPhase string

const (
	// ElasticsearchReadyPhase is operating at the desired spec.
	ElasticsearchReadyPhase ElasticsearchOrchestrationPhase = "Ready"
	// ElasticsearchApplyingChangesPhase controller is working towards a desired state, cluster can be unavailable.
	ElasticsearchApplyingChangesPhase ElasticsearchOrchestrationPhase = "ApplyingChanges"
	// ElasticsearchMigratingDataPhase Elasticsearch is currently migrating data to another node.
	ElasticsearchMigratingDataPhase ElasticsearchOrchestrationPhase = "MigratingData"
	// ElasticsearchNodeShutdownStalledPhase Elasticsearch cannot make progress with a node shutdown during downscale or rolling upgrade.
	ElasticsearchNodeShutdownStalledPhase ElasticsearchOrchestrationPhase = "Stalled"
	// ElasticsearchResourceInvalid is marking a resource as invalid, should never happen if admission control is installed correctly.
	ElasticsearchResourceInvalid ElasticsearchOrchestrationPhase = "Invalid"
)

// ElasticsearchStatus represents the observed state of Elasticsearch.
type ElasticsearchStatus struct {
	// AvailableNodes is the number of available instances.
	AvailableNodes int32 `json:"availableNodes,omitempty"`
	// Version of the stack resource currently running. During version upgrades, multiple versions may run
	// in parallel: this value specifies the lowest version currently running.
	Version string                          `json:"version,omitempty"`
	Health  ElasticsearchHealth             `json:"health,omitempty"`
	Phase   ElasticsearchOrchestrationPhase `json:"phase,omitempty"`

	MonitoringAssociationsStatus commonv1.AssociationStatusMap `json:"monitoringAssociationStatus,omitempty"`

	// +optional
	// Conditions holds the current service state of an Elasticsearch cluster.
	// **This API is in technical preview and may be changed or removed in a future release.**
	Conditions Conditions `json:"conditions"`

	// +optional
	// InProgressOperations represents changes being applied by the operator to the Elasticsearch cluster.
	// **This API is in technical preview and may be changed or removed in a future release.**
	InProgressOperations `json:"inProgressOperations"`

	// ObservedGeneration is the most recent generation observed for this Elasticsearch cluster.
	// It corresponds to the metadata generation, which is updated on mutation by the API Server.
	// If the generation observed in status diverges from the generation in metadata, the Elasticsearch
	// controller has not yet processed the changes contained in the Elasticsearch specification.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// IsDegraded returns true if the current status is worse than the previous.
func (es ElasticsearchStatus) IsDegraded(prev ElasticsearchStatus) bool {
	return es.Health.Less(prev.Health)
}

func (es *Elasticsearch) AssociationStatusMap(typ commonv1.AssociationType) commonv1.AssociationStatusMap {
	if typ != commonv1.EsMonitoringAssociationType {
		return commonv1.AssociationStatusMap{}
	}

	return es.Status.MonitoringAssociationsStatus
}

func (es *Elasticsearch) SetAssociationStatusMap(typ commonv1.AssociationType, status commonv1.AssociationStatusMap) error {
	if typ != commonv1.EsMonitoringAssociationType {
		return fmt.Errorf("association type %s not known", typ)
	}

	es.Status.MonitoringAssociationsStatus = status
	return nil
}

// ConditionType defines the condition of an Elasticsearch resource.
type ConditionType string

const (
	ElasticsearchIsReachable ConditionType = "ElasticsearchIsReachable"
	ReconciliationComplete   ConditionType = "ReconciliationComplete"
	RunningDesiredVersion    ConditionType = "RunningDesiredVersion"
)

// Condition represents Elasticsearch resource's condition.
// **This API is in technical preview and may be changed or removed in a future release.**
type Condition struct {
	Type   ConditionType          `json:"type"`
	Status corev1.ConditionStatus `json:"status"`
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// +optional
	Message string `json:"message,omitempty"`
}

type Conditions []Condition

func (c Conditions) Index(conditionType ConditionType) int {
	for i, condition := range c {
		if condition.Type == conditionType {
			return i
		}
	}
	return -1
}

func (c Conditions) MergeWith(nextCondition Condition) Conditions {
	cp := c.DeepCopy()
	if index := cp.Index(nextCondition.Type); index >= 0 {
		currentCondition := c[index]
		if currentCondition.Status != nextCondition.Status ||
			currentCondition.Message != nextCondition.Message {
			// Update condition
			cp[index] = nextCondition
		}
	} else {
		cp = append(cp, nextCondition)
	}
	return cp
}

// NewNodeStatus provides details about the status of nodes which are expected to be created and added to the Elasticsearch cluster.
// **This API is in technical preview and may be changed or removed in a future release.**
type NewNodeStatus string

const (
	// NewNodePending surfaces a situation where a node creation is delayed.
	NewNodePending NewNodeStatus = "PENDING"

	// NewNodeExpected states that the node is expected to be created because the underlying StatefulSet has been upscaled.
	NewNodeExpected NewNodeStatus = "EXPECTED"
)

type NewNode struct {
	// Name of the Elasticsearch node that should be added to the cluster.
	Name string `json:"name"`

	// NewNodeStatus states if a new node is being created, or if the upscale is delayed.
	Status NewNodeStatus `json:"status"`

	// +optional
	// Optional message to explain why a node may not be immediately added.
	Message *string `json:"message,omitempty"`
}

// UpscaleOperation provides an overview of in progress changes applied by the operator to add Elasticsearch nodes to the cluster.
// **This API is in technical preview and may be changed or removed in a future release.**
type UpscaleOperation struct {
	LastUpdatedTime metav1.Time `json:"lastUpdatedTime,omitempty"`
	// Nodes expected to be added by the operator.
	Nodes []NewNode `json:"nodes,omitempty"`
}

// UpgradedNode provides details about the status of nodes which are expected to be updated.
// **This API is in technical preview and may be changed or removed in a future release.**
type UpgradedNode struct {
	// Name of the Elasticsearch node that should be upgraded.
	Name string `json:"name"`

	// Status states if the node is either in the process of being deleted for an upgrade,
	// or blocked by a predicate or another condition stated in the message field.
	Status string `json:"status"`

	// +optional
	// Optional message to explain why a node may not be immediately restarted for upgrade.
	Message *string `json:"message,omitempty"`

	// Predicate is the name of the predicate currently preventing this node from being deleted for an upgrade.
	// +optional
	Predicate *string `json:"predicate,omitempty"`
}

// UpgradeOperation provides an overview of the pending or in progress changes applied by the operator to update the Elasticsearch nodes in the cluster.
// **This API is in technical preview and may be changed or removed in a future release.**
type UpgradeOperation struct {
	LastUpdatedTime metav1.Time `json:"lastUpdatedTime,omitempty"`

	// Nodes that must be restarted for upgrade.
	Nodes []UpgradedNode `json:"nodes,omitempty"`
}

// DownscaledNode provides an overview of in progress changes applied by the operator to remove Elasticsearch nodes from the cluster.
// **This API is in technical preview and may be changed or removed in a future release.**
type DownscaledNode struct {
	// Name of the Elasticsearch node that should be removed.
	Name string `json:"name"`

	// Shutdown status as returned by the Elasticsearch shutdown API.
	// If the Elasticsearch shutdown API is not available, the shutdown status is then inferred from the remaining
	// shards on the nodes, as observed by the operator.
	ShutdownStatus string `json:"shutdownStatus"`

	// +optional
	// Explanation provides details about an in progress node shutdown. It is only available for clusters managed with the
	// Elasticsearch shutdown API.
	Explanation *string `json:"explanation,omitempty"`
}

// DownscaleOperation provides details about in progress downscale operations.
// **This API is in technical preview and may be changed or removed in a future release.**
type DownscaleOperation struct {
	LastUpdatedTime metav1.Time `json:"lastUpdatedTime,omitempty"`

	// Nodes which are scheduled to be removed from the cluster.
	Nodes []DownscaledNode `json:"nodes,omitempty"`

	// Stalled represents a state where no progress can be made.
	// It is only available for clusters managed with the Elasticsearch shutdown API.
	// +optional
	Stalled *bool `json:"stalled,omitempty"`
}

// InProgressOperations provides details about in progress changes applied by the operator on the Elasticsearch cluster.
// **This API is in technical preview and may be changed or removed in a future release.**
type InProgressOperations struct {
	DownscaleOperation DownscaleOperation `json:"downscale"`
	UpgradeOperation   UpgradeOperation   `json:"upgrade"`
	UpscaleOperation   UpscaleOperation   `json:"upscale"`
}
