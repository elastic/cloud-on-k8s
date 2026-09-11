// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// directElasticsearchRef is the ElasticsearchRef resolver for associations that reference
// Elasticsearch directly: it returns the association's own ref unchanged.
func directElasticsearchRef(_ context.Context, _ k8s.Client, assoc commonv1.Association) (bool, commonv1.AssociationRef, error) {
	return true, assoc.AssociationRef(), nil
}

// getElasticsearchFromKibana returns the Elasticsearch reference in which the user must be created for this association.
func getElasticsearchFromKibana(ctx context.Context, c k8s.Client, association commonv1.Association) (bool, commonv1.AssociationRef, error) {
	kibanaRef := association.AssociationRef()
	if !kibanaRef.IsSet() {
		return false, commonv1.ObjectSelector{}, nil
	}

	kb := kbv1.Kibana{}
	err := c.Get(ctx, kibanaRef.NamespacedName(), &kb)
	if errors.IsNotFound(err) {
		return false, commonv1.ObjectSelector{}, nil
	}
	if err != nil {
		return false, commonv1.ObjectSelector{}, err
	}

	esRef := kb.EsAssociation().AssociationRef()
	if !esRef.IsSet() {
		return false, commonv1.ObjectSelector{}, nil
	}
	return true, esRef, nil
}
