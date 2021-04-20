// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"context"
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ServiceURL calculates the URL for the service identified by serviceNSN using protocol.
func ServiceURL(c k8s.Client, serviceNSN types.NamespacedName, protocol string) (string, error) {
	var svc corev1.Service
	if err := c.Get(context.Background(), serviceNSN, &svc); err != nil {
		return "", fmt.Errorf("while fetching refernced service: %w", err)
	}
	port, err := findPortFor(protocol, svc)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s://%s.%s.svc:%d", protocol, svc.Name, svc.Namespace, port), nil
}

// findPortFor returns the port with the name matching protocol.
func findPortFor(protocol string, svc corev1.Service) (int32, error) {
	for _, p := range svc.Spec.Ports {
		if p.Name == protocol {
			return p.Port, nil
		}
	}
	return -1, fmt.Errorf("no port named [%s] in service [%s/%s]", protocol, svc.Namespace, svc.Name)
}

// filterWithServiceName returns those associations that have a serviceName specified.
func filterWithServiceName(associations []commonv1.Association) []commonv1.Association {
	var r []commonv1.Association
	for _, a := range associations {
		if a.AssociationRef().ServiceName != "" {
			r = append(r, a)
		}
	}
	return r
}
