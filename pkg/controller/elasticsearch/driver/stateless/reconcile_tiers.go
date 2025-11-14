// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateless

import (
	"context"
	"fmt"
	"time"

	"github.com/openkruise/kruise-api/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (sd *statelessDriver) reconcileTiers(
	ctx context.Context,
	expectations *expectations.Expectations,
	meta metadata.Metadata,
) *reconciler.Results {
	results := reconciler.NewResult(ctx)

	// check if actual CloneSets match our expectations before applying any change
	ok, reason, err := sd.expectationsSatisfied(ctx)
	if err != nil {
		return results.WithError(err)
	}
	if !ok {
		return results.WithReconciliationState(reconciler.Requeue.WithReason(reason))
	}

	ver, err := version.Parse(sd.ES.Spec.Version)
	if err != nil {
		return results.WithError(err)
	}

	secretSettings, err := sd.getSecretSettings(ctx)
	if err != nil {
		return results.WithError(err)
	}

	for _, tier := range esv1.AllElasticsearchTierNames {
		meta := meta.Merge(metadata.Metadata{Labels: map[string]string{
			label.TierLabelName: string(tier),
		}})

		tierSpec, err := sd.ES.GetTierSpec(tier)
		if err != nil {
			results.WithError(err)
			continue
		}

		// Build ES configuration.
		userCfg := commonv1.Config{}
		if tierSpec.Config != nil {
			userCfg = *tierSpec.Config
		}

		// Get Policy config
		policyConfig, err := nodespec.GetPolicyConfig(ctx, sd.Client, sd.ES)
		if err != nil {
			results.WithError(err)
			continue
		}
		cfg, err := settings.NewMergedESConfig(
			sd.ES.Name, true, ver,
			sd.OperatorParameters.IPFamily,
			sd.ES.Spec.HTTP, userCfg,
			policyConfig.ElasticsearchConfig,
			false, /* Spec.RemoteClusterServer.Enabled */
			sd.OperatorParameters.SetDefaultSecurityContext,
		)
		if err != nil {
			results.WithError(err)
			continue
		}

		// Add stateless specific config
		cfg, err = settings.WithStatelessConfig(tier, sd.ES.Spec.StatelessSpec.StatelessConfig.ObjectStore, cfg)
		if err != nil {
			results.WithError(err)
			continue
		}

		// Get existing secure settings secret to possibly reuse its version.
		var existing corev1.Secret
		if err := sd.Client.Get(ctx, client.ObjectKey{
			Name:      settings.ConfigSecretName(esv1.PodsControllerResourceName(sd.ES.Name, string(tier))),
			Namespace: sd.ES.Namespace,
		}, &existing); err != nil && !errors.IsNotFound(err) {
			results.WithError(err)
			continue
		}

		// Build Settings Secret
		newSecureSettingsVersion := fmt.Sprintf("%d", time.Now().Unix())
		secureSettings := settings.NewSecureSettings(existing, newSecureSettingsVersion, secretSettings)
		cloneSetName := esv1.PodsControllerResourceName(sd.ES.Name, string(tier))
		operatorPrivilegesSettings, err := settings.NewOperatorPrivilegesSettings(
			[]settings.OperatorAccount{
				{
					Names:     []string{user.ControllerUserName},
					RealmType: settings.OperatorRealmTypeFile,
				},
			}, nil)
		if err != nil {
			results.WithError(err)
			continue
		}
		if err := settings.ReconcileConfig(ctx, sd.Client, sd.ES, cloneSetName, cfg, meta, secureSettings, operatorPrivilegesSettings); err != nil {
			results.WithError(err)
			continue
		}

		// Check if a volume claim template is specified, otherwise use default.
		volumeClaimTemplate := tierSpec.VolumeClaimTemplate.ToPersistentVolumeClaimTemplate()

		// Update VolumeClaimTemplate metadata.
		volumeClaimTemplateMeta := meta.Merge(
			metadata.Metadata{
				Labels:      volumeClaimTemplate.Labels,
				Annotations: volumeClaimTemplate.Annotations,
			},
		)
		volumeClaimTemplate.Labels = volumeClaimTemplateMeta.Labels
		volumeClaimTemplate.Annotations = volumeClaimTemplateMeta.Annotations

		// cloneSetSelector is used to match the cloneSetSelector pods
		cloneSetSelector := label.NewCloneSetLabels(k8s.ExtractNamespacedName(&sd.ES), cloneSetName)
		mergedMeta := meta.Merge(metadata.Metadata{Labels: cloneSetSelector})

		// Pod template
		podTemplateSpec, err := nodespec.BuildPodTemplateSpec(
			ctx,
			sd.Client,
			sd.ES,
			tierSpec.AsNamedTierSpec(tier),
			cfg,
			nil, /* No keystore resources, we use secure file settings instead */
			sd.OperatorParameters.SetDefaultSecurityContext,
			policyConfig,
			meta,
		)
		if err != nil {
			results.WithError(err)
			return nil
		}

		// Add the volume spec hash to the pod template for change detection:
		// > Note: If you only change the volumeClaimTemplates field, the Pod upgrade will not be triggered,
		// > you need to trigger the rolling upgrade by changing Labels, Annotations, Image, Env, etc.
		if podTemplateSpec.Annotations == nil {
			podTemplateSpec.Annotations = make(map[string]string)
		}
		podTemplateSpec.Annotations["elasticsearch.k8s.elastic.co/volume-claim-template-hash"] = hash.HashObject(volumeClaimTemplate.Spec)

		// Expected CloneSet
		expected := v1alpha1.CloneSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:        cloneSetName,
				Namespace:   sd.ES.Namespace,
				Labels:      mergedMeta.Labels,
				Annotations: mergedMeta.Annotations,
			},
			Spec: v1alpha1.CloneSetSpec{
				UpdateStrategy: v1alpha1.CloneSetUpdateStrategy{
					MaxUnavailable: ptr.To(intstr.FromInt32(0)),
					MaxSurge:       ptr.To(intstr.FromInt32(1)),
					Type:           v1alpha1.RecreateCloneSetUpdateStrategyType,
				},
				ScaleStrategy: v1alpha1.CloneSetScaleStrategy{
					DisablePVCReuse: true,
				},
				Selector: &metav1.LabelSelector{
					MatchLabels: cloneSetSelector,
				},
				Replicas:             ptr.To(tierSpec.Count),
				Template:             podTemplateSpec,
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{*volumeClaimTemplate},
			},
		}

		expected = WithTemplateHash(expected)
		reconciled := &v1alpha1.CloneSet{}
		results.WithError(reconciler.ReconcileResource(reconciler.Params{
			Context:    ctx,
			Client:     sd.Client,
			Owner:      &sd.ES,
			Expected:   &expected,
			Reconciled: reconciled,
			NeedsUpdate: func() bool {
				// expected labels or annotations not there
				return !maps.IsSubset(expected.Labels, reconciled.Labels) ||
					!maps.IsSubset(expected.Annotations, reconciled.Annotations) ||
					// different spec
					!(expected.Labels[hash.TemplateHashLabelName] == reconciled.Labels[hash.TemplateHashLabelName])
			},
			UpdateReconciled: func() {
				// set expected annotations and labels, but don't remove existing ones
				// that may have been defaulted or set by a user/admin on the existing resource
				reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
				reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
				// overwrite the spec but leave the status intact
				reconciled.Spec = expected.Spec
			},
			PostUpdate: func() {
				if expectations != nil {
					// expect the reconciled StatefulSet to be there in the cache for next reconciliations,
					// to prevent assumptions based on the wrong replica count
					expectations.ExpectGeneration(reconciled)
				}
			},
		}))
	}
	return results
}

// WithTemplateHash returns a new CloneSet with a hash of its template to ease comparisons.
func WithTemplateHash(cs v1alpha1.CloneSet) v1alpha1.CloneSet {
	csCopy := *cs.DeepCopy()
	csCopy.Labels = hash.SetTemplateHashLabel(csCopy.Labels, csCopy)
	return csCopy
}
