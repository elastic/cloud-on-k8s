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
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/filesettings"
	eslabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ElasticSearchConfigKey            = "elasticsearch.json"
	SecretsMountKey                   = "secretMounts.json"
	ElasticsearchConfigHashAnnotation = "policy.k8s.elastic.co/elasticsearch-config-hash"
	SecretMountsHashAnnotation        = "policy.k8s.elastic.co/secret-mounts-hash"
)

func newElasticsearchConfigSecret(policy policyv1alpha1.StackConfigPolicy, es esv1.Elasticsearch) (corev1.Secret, error) {
	secretMountBytes, err := json.Marshal(policy.Spec.Elasticsearch.SecretMounts)
	if err != nil {
		return corev1.Secret{}, err
	}
	secretsMountHash := hash.HashObject(policy.Spec.Elasticsearch.SecretMounts)

	var configDataJsonBytes []byte
	var elasticsearchConfigHash string
	if policy.Spec.Elasticsearch.Config != nil {
		configDataJsonBytes, err = policy.Spec.Elasticsearch.Config.MarshalJSON()
		if err != nil {
			return corev1.Secret{}, err
		}
		elasticsearchConfigHash = hash.HashObject(policy.Spec.Elasticsearch.Config)
	}

	elasticsearchConfigSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      esv1.StakConfigElasticsearchConfigSecretName(es.Name),
			Labels: eslabel.NewLabels(types.NamespacedName{
				Name:      es.Name,
				Namespace: es.Namespace,
			}),
			Annotations: map[string]string{
				ElasticsearchConfigHashAnnotation: elasticsearchConfigHash,
				SecretMountsHashAnnotation:        secretsMountHash,
			},
		},
		Data: map[string][]byte{
			ElasticSearchConfigKey: configDataJsonBytes,
			SecretsMountKey:        secretMountBytes,
		},
	}

	// Set Elasticsearch as the soft owner
	filesettings.SetSoftOwner(&elasticsearchConfigSecret, policy)

	// Add label to delete secret on deletion of the stack config policy
	elasticsearchConfigSecret.Labels[eslabel.StackConfigPolicyOnDeleteLabelName] = "delete"

	return elasticsearchConfigSecret, nil
}

func reconcileSecret(ctx context.Context,
	c k8s.Client,
	expected corev1.Secret,
	owner client.Object) error {
	reconciled := &corev1.Secret{}
	return reconciler.ReconcileResource(reconciler.Params{
		Context:    ctx,
		Client:     c,
		Owner:      owner,
		Expected:   &expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			return !maps.IsSubset(expected.Labels, reconciled.Labels) ||
				!maps.IsSubset(expected.Annotations, reconciled.Annotations) ||
				!reflect.DeepEqual(expected.Data, reconciled.Data)
		},
		UpdateReconciled: func() {
			reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
			reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
			reconciled.Data = expected.Data
		},
	})
}

// reconcileSecretMountSecretsESNamespace creates the secrets in SecretMounts to the respective Elasticsearch namespace where they should be mounted to.
func reconcileSecretMountSecretsESNamespace(ctx context.Context, c k8s.Client, es esv1.Elasticsearch, policy *policyv1alpha1.StackConfigPolicy) error {
	for _, secretMount := range policy.Spec.Elasticsearch.SecretMounts {
		additionalSecret := corev1.Secret{}
		namespacedName := types.NamespacedName{
			Name:      secretMount.SecretName,
			Namespace: policy.Namespace,
		}
		if err := c.Get(ctx, namespacedName, &additionalSecret); err != nil {
			return err
		}

		// Recreate it in the Elasticsearch namespace, prefix with es name.
		secretName := esv1.ESNamer.Suffix(es.Name, additionalSecret.Name)
		expected := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: es.Namespace,
				Name:      secretName,
				Labels: eslabel.NewLabels(types.NamespacedName{
					Name:      es.Name,
					Namespace: es.Namespace,
				}),
			},
			Data: additionalSecret.Data,
		}

		// Set stackconfigpolicy as a softowner
		filesettings.SetSoftOwner(&expected, *policy)

		// Set the secret to be deleted when the stack config policy is deleted.
		expected.Labels[eslabel.StackConfigPolicyOnDeleteLabelName] = "delete"

		err := reconcileSecret(ctx, c, expected, nil)
		if err != nil {
			return err
		}
	}
	return nil
}
