// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"context"
	"encoding/json"
	"reflect"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	eslabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	elasticSearchConfigKey = "elasticsearch.yml"
	secretsMountKey        = "secretMounts.json"
)

var (
	managedLabels = []string{reconciler.SoftOwnerNamespaceLabel, reconciler.SoftOwnerNameLabel, reconciler.SoftOwnerKindLabel}
)

func NewElasticsearchConfigSecret(policy policyv1alpha1.StackConfigPolicy, es esv1.Elasticsearch) (corev1.Secret, error) {
	secretMountBytes, err := json.Marshal(policy.Spec.Elasticsearch.SecretMounts)
	if err != nil {
		return corev1.Secret{}, err
	}

	var configDataJsonBytes []byte
	if policy.Spec.Elasticsearch.Config != nil {
		configDataJsonBytes, err = policy.Spec.Elasticsearch.Config.MarshalJSON()
		if err != nil {
			return corev1.Secret{}, err
		}
	}

	elasticsearchConfigSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      esv1.StakConfigElasticsearchConfigSecretName(es.Name),
			Labels: eslabel.NewLabels(types.NamespacedName{
				Name:      es.Name,
				Namespace: es.Namespace,
			}),
		},
		Data: map[string][]byte{
			elasticSearchConfigKey: configDataJsonBytes,
			secretsMountKey:        secretMountBytes,
		},
	}

	// Set the Elasticsearch as the soft owner
	setPolicyAsSoftOwner(&elasticsearchConfigSecret, policy)

	// Add label to delete secret on deletion of the stack config policy
	elasticsearchConfigSecret.Labels[label.StackConfigPolicyOnDeleteLabelName] = "delete"

	return elasticsearchConfigSecret, nil
}

func ReconcileElasticsearchConfigSecret(ctx context.Context,
	c k8s.Client,
	expected corev1.Secret,
	es esv1.Elasticsearch,
	policy policyv1alpha1.StackConfigPolicy) error {
	reconciled := &corev1.Secret{}
	return reconciler.ReconcileResource(reconciler.Params{
		Context:    ctx,
		Client:     c,
		Owner:      &es,
		Expected:   &expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			return !maps.IsSubset(expected.Labels, reconciled.Labels) ||
				!maps.IsSubset(expected.Annotations, reconciled.Annotations) ||
				!reflect.DeepEqual(expected.Data, reconciled.Data)
		},
		UpdateReconciled: func() {
			reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
			// remove managed labels if they are no longer defined
			for _, label := range managedLabels {
				if _, ok := expected.Labels[label]; !ok {
					delete(reconciled.Labels, label)
				}
			}
			reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
			reconciled.Data = expected.Data
		},
	})
}

// setSoftOwner sets the given stack config policy as soft owner of the Secret using the "softOwned" labels.
func setPolicyAsSoftOwner(secret *corev1.Secret, policy policyv1alpha1.StackConfigPolicy) {
	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}
	secret.Labels[reconciler.SoftOwnerNamespaceLabel] = policy.GetNamespace()
	secret.Labels[reconciler.SoftOwnerNameLabel] = policy.GetName()
	secret.Labels[reconciler.SoftOwnerKindLabel] = policy.GetObjectKind().GroupVersionKind().Kind
}
