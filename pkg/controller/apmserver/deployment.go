// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	"context"
	"fmt"
	"hash/fnv"

	"go.elastic.co/apm/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func (r *ReconcileApmServer) reconcileApmServerDeployment(ctx context.Context, state State, as *apmv1.ApmServer, version version.Version) (State, error) {
	span, ctx := apm.StartSpan(ctx, "reconcile_deployment", tracing.SpanTypeApp)
	defer span.End()

	tokenSecret, err := reconcileApmServerToken(ctx, r.Client, as)
	if err != nil {
		return state, err
	}
	reconciledConfigSecret, err := reconcileApmServerConfig(ctx, r.Client, as, version)
	if err != nil {
		return state, err
	}

	keystoreResources, err := keystore.ReconcileResources(
		ctx,
		r,
		as,
		Namer,
		as.GetIdentityLabels(),
		initContainerParameters,
	)
	if err != nil {
		return state, err
	}

	apmServerPodSpecParams := PodSpecParams{
		Version:         as.Spec.Version,
		CustomImageName: as.Spec.Image,

		PodTemplate: as.Spec.PodTemplate,

		TokenSecret:  tokenSecret,
		ConfigSecret: reconciledConfigSecret,

		keystoreResources: keystoreResources,
	}
	params, err := r.deploymentParams(as, apmServerPodSpecParams)
	if err != nil {
		return state, err
	}

	deploy := deployment.New(params)
	result, err := deployment.Reconcile(ctx, r.K8sClient(), deploy, as)
	if err != nil {
		return state, err
	}

	pods, err := k8s.PodsMatchingLabels(r.K8sClient(), as.Namespace, map[string]string{ApmServerNameLabelName: as.Name})
	if err != nil {
		return state, err
	}
	if err := state.UpdateApmServerState(ctx, result, pods, tokenSecret); err != nil {
		return state, err
	}
	return state, nil
}

func (r *ReconcileApmServer) deploymentParams(
	as *apmv1.ApmServer,
	params PodSpecParams,
) (deployment.Params, error) {
	podSpec, err := newPodSpec(r.Client, as, params)
	if err != nil {
		return deployment.Params{}, err
	}

	return deployment.Params{
		Name:                 Deployment(as.Name),
		Namespace:            as.Namespace,
		Replicas:             as.Spec.Count,
		Selector:             as.GetIdentityLabels(),
		Labels:               as.GetIdentityLabels(),
		RevisionHistoryLimit: as.Spec.RevisionHistoryLimit,
		PodTemplateSpec:      podSpec,
		Strategy:             appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
	}, nil
}

func buildConfigHash(c k8s.Client, as *apmv1.ApmServer, params PodSpecParams) (string, error) {
	// build a hash of various settings to rotate the Pod on any change
	configHash := fnv.New32a()

	// - in the APMServer configuration file content
	_, _ = configHash.Write(params.ConfigSecret.Data[ApmCfgSecretKey])

	// - in the APMServer keystore
	if params.keystoreResources != nil {
		_, _ = configHash.Write([]byte(params.keystoreResources.Hash))
	}

	// - in the APMServer TLS certificates
	if as.Spec.HTTP.TLS.Enabled() {
		var tlsCertSecret corev1.Secret
		tlsSecretKey := types.NamespacedName{Namespace: as.Namespace, Name: certificates.InternalCertsSecretName(Namer, as.Name)}
		if err := c.Get(context.Background(), tlsSecretKey, &tlsCertSecret); err != nil {
			return "", err
		}
		if certPem, ok := tlsCertSecret.Data[certificates.CertFileName]; ok {
			_, _ = configHash.Write(certPem)
		}
	}

	// - in the CA certificates of the referenced resources in associations
	for _, association := range as.GetAssociations() {
		assocConf, err := association.AssociationConf()
		if err != nil {
			return "", err
		}
		if assocConf.CAIsConfigured() {
			var publicCASecret corev1.Secret
			key := types.NamespacedName{Namespace: as.Namespace, Name: assocConf.GetCASecretName()}
			if err := c.Get(context.Background(), key, &publicCASecret); err != nil {
				return "", err
			}
			if certPem, ok := publicCASecret.Data[certificates.CAFileName]; ok {
				_, _ = configHash.Write(certPem)
			}
		}
	}

	return fmt.Sprint(configHash.Sum32()), nil
}
