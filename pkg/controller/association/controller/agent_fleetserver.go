// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/agent"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/rbac"
)

func AddAgentFleetServer(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate:     func() commonv1.Associated { return &agentv1alpha1.Agent{} },
		ReferencedObjTemplate:     func() client.Object { return &agentv1alpha1.Agent{} },
		ExternalServiceURL:        getFleetServerExternalURL,
		ReferencedResourceVersion: referencedFleetServerStatusVersion,
		ReferencedResourceNamer:   agent.Namer,
		AssociationName:           "agent-fleetserver",
		AssociatedShortName:       "agent",
		AssociationType:           commonv1.FleetServerAssociationType,
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				AgentAssociationLabelName:      associated.Name,
				AgentAssociationLabelNamespace: associated.Namespace,
				AgentAssociationLabelType:      commonv1.FleetServerAssociationType,
			}
		},
		AssociationConfAnnotationNameBase:     commonv1.FleetServerConfigAnnotationNameBase,
		AssociationResourceNameLabelName:      agent.NameLabelName,
		AssociationResourceNamespaceLabelName: agent.NamespaceLabelName,

		ElasticsearchUserCreation: nil,
	})
}

func getFleetServerExternalURL(c k8s.Client, assoc commonv1.Association) (string, error) {
	fleetServerRef := assoc.AssociationRef()
	if !fleetServerRef.IsDefined() {
		return "", nil
	}
	fleetServer := agentv1alpha1.Agent{}
	if err := c.Get(context.Background(), fleetServerRef.NamespacedName(), &fleetServer); err != nil {
		return "", err
	}
	serviceName := fleetServerRef.ServiceName
	if serviceName == "" {
		serviceName = agent.HTTPServiceName(fleetServer.Name)
	}
	nsn := types.NamespacedName{Namespace: fleetServer.Namespace, Name: serviceName}
	return association.ServiceURL(c, nsn, fleetServer.Spec.HTTP.Protocol())
}

// referencedFleetServerStatusVersion returns the currently running version of Agent
// reported in its status.
func referencedFleetServerStatusVersion(c k8s.Client, fsRef commonv1.ObjectSelector) (string, error) {
	if fsRef.IsExternal() {
		info, err := association.GetUnmanagedAssociationConnectionInfoFromSecret(c, fsRef)
		if err != nil {
			return "", err
		}
		ver, err := info.Version("/api/status", "{ .version.number }")
		if err != nil {
			// version is in the status API from version 8.0
			if err.Error() == "version is not found" {
				return association.UnknownVersion, nil
			}
			return "", err
		}
		return ver, nil
	}

	var fleetServer agentv1alpha1.Agent
	err := c.Get(context.Background(), fsRef.NamespacedName(), &fleetServer)
	if err != nil {
		return "", err
	}
	return fleetServer.Status.Version, nil
}
