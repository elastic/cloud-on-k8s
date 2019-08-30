// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
)

// UserFinalizer ensures that any external user created for an associated object is removed.
func UserFinalizer(c k8s.Client, selector labels.Selector) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: "users.finalizers.associations.k8s.elastic.co",
		Execute: func() error {
			var secrets corev1.SecretList
			// TODO sabo fix this and actually use the label selector
			if err := c.List(&secrets); err != nil {
				// if err := c.List(&client.ListOptions{LabelSelector: selector}, &secrets); err != nil {
				return err
			}
			for _, s := range secrets.Items {
				if err := c.Delete(&s); err != nil && !apierrors.IsNotFound(err) {
					return err
				}
			}
			return nil
		},
	}
}
