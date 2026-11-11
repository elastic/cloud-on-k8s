// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build integration

package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/elastic/cloud-on-k8s/v3/pkg/about"
	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nsmatch"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/license/trial"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/test"
)

const operatorNamespace = "elastic-system"

func TestMain(m *testing.M) {
	test.RunWithK8s(m)
}

// TestFleetServerClientTrustBundleFollowsTrustSources tests that the client certificate trust bundle of a Fleet
// Server requiring client authentication is rebuilt when a client certificate trust source is published for it, and
// that only the trust sources the Fleet Server controls in its own namespace are trusted.
func TestFleetServerClientTrustBundleFollowsTrustSources(t *testing.T) {
	const (
		agentNamespace = "fleet-trust-ns"
		otherNamespace = "fleet-trust-other-ns"
		agentName      = "fleet-server"
	)
	ctx := context.Background()

	params := operator.Parameters{
		OperatorNamespace: operatorNamespace,
		OperatorInfo:      about.OperatorInfo{BuildInfo: about.BuildInfo{Version: "3.5.0"}},
		IPFamily:          corev1.IPv4Protocol,
		CACertRotation:    certificates.RotationParams{Validity: certificates.DefaultCertValidity, RotateBefore: certificates.DefaultRotateBefore},
		CertRotation:      certificates.RotationParams{Validity: certificates.DefaultCertValidity, RotateBefore: certificates.DefaultRotateBefore},
		NamespaceMatcher:  nsmatch.NewNamespaceMatcher(nil, operatorNamespace),
	}

	c, stop := test.StartManager(t, func(mgr manager.Manager, p operator.Parameters) error {
		// Fleet Server client authentication is an Enterprise feature: run the trial controller so that the trial
		// license created below becomes a valid ECK-managed trial.
		if err := trial.Add(mgr, p); err != nil {
			return err
		}
		return Add(mgr, p)
	}, params)
	defer stop()

	for _, ns := range []string{operatorNamespace, agentNamespace, otherNamespace} {
		require.NoError(t, test.EnsureNamespace(c, ns))
	}

	require.NoError(t, license.CreateTrialLicense(ctx, c, types.NamespacedName{Namespace: operatorNamespace, Name: "eck-trial-license"}))
	checker := license.NewLicenseChecker(c, operatorNamespace)
	test.RetryUntilSuccess(t, func() error {
		enabled, err := checker.EnterpriseFeaturesEnabled(ctx)
		if err != nil {
			return err
		}
		if !enabled {
			return errors.New("enterprise features not enabled yet")
		}
		return nil
	})

	fleetServer := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: agentName, Namespace: agentNamespace},
		Spec: agentv1alpha1.AgentSpec{
			Version:            "9.5.0",
			Mode:               agentv1alpha1.AgentFleetMode,
			FleetServerEnabled: true,
			Deployment:         &agentv1alpha1.DeploymentSpec{},
			HTTP: commonv1.HTTPConfigWithClientOptions{
				TLS: commonv1.TLSWithClientOptions{Client: commonv1.ClientOptions{Authentication: true}},
			},
		},
	}
	require.NoError(t, c.Create(ctx, fleetServer))

	bundleKey := types.NamespacedName{Namespace: agentNamespace, Name: certificates.ClientCertTrustBundleSecretName(Namer, agentName)}
	bundle := func() (string, error) {
		var secret corev1.Secret
		if err := c.Get(ctx, bundleKey, &secret); err != nil {
			return "", err
		}
		return string(secret.Data[certificates.ClientCertificatesTrustBundleFileName]), nil
	}
	test.RetryUntilSuccess(t, func() error {
		_, err := bundle()
		return err
	})

	// the trust sources, controlled or not by the Fleet Server, in its namespace or not
	var created agentv1alpha1.Agent
	require.NoError(t, c.Get(ctx, k8s.ExtractNamespacedName(fleetServer), &created))
	ownerRef := *metav1.NewControllerRef(&created, agentv1alpha1.GroupVersion.WithKind(agentv1alpha1.Kind))
	otherOwnerRef := ownerRef
	otherOwnerRef.UID = "another-agent-uid"
	rawTrustSource := func(namespace, name, cert string, withType bool, owner *metav1.OwnerReference) *corev1.Secret {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: map[string]string{}},
			Data:       map[string][]byte{certificates.CertFileName: []byte(cert)},
		}
		if withType {
			secret.Labels[commonv1.TypeLabelName] = certificates.ClientCertTrustSourceType
		}
		if owner != nil {
			secret.OwnerReferences = []metav1.OwnerReference{*owner}
		}
		return secret
	}

	trustSources := []struct {
		name string
		// publish creates the trust source.
		publish func(t *testing.T)
		cert    string
		// wantTrusted is whether the Fleet Server trusts the client certificate.
		wantTrusted bool
	}{
		{
			name: "trust source in another namespace pointing at the Fleet Server",
			cert: "CERT-OTHER-NAMESPACE",
			publish: func(t *testing.T) {
				t.Helper()
				require.NoError(t, c.Create(ctx, rawTrustSource(otherNamespace, "trust-source-in-other-namespace", "CERT-OTHER-NAMESPACE", true, &ownerRef)))
			},
		},
		{
			name: "trust source without owner",
			cert: "CERT-UNOWNED",
			publish: func(t *testing.T) {
				t.Helper()
				require.NoError(t, c.Create(ctx, rawTrustSource(agentNamespace, "unowned-trust-source", "CERT-UNOWNED", true, nil)))
			},
		},
		{
			name: "trust source controlled by another resource",
			cert: "CERT-OTHER-OWNER",
			publish: func(t *testing.T) {
				t.Helper()
				require.NoError(t, c.Create(ctx, rawTrustSource(agentNamespace, "trust-source-of-other-owner", "CERT-OTHER-OWNER", true, &otherOwnerRef)))
			},
		},
		{
			name: "secret controlled by the Fleet Server which is not a trust source",
			cert: "CERT-UNTYPED",
			publish: func(t *testing.T) {
				t.Helper()
				require.NoError(t, c.Create(ctx, rawTrustSource(agentNamespace, "not-a-trust-source", "CERT-UNTYPED", false, &ownerRef)))
			},
		},
		{
			name:        "trust source controlled by the Fleet Server in its namespace",
			cert:        "CERT-OWNED-TRUST-SOURCE",
			wantTrusted: true,
			publish: func(t *testing.T) {
				t.Helper()
				// published as the association controllers do
				require.NoError(t, certificates.ReconcileClientCertTrustSource(
					ctx, c, &created, types.NamespacedName{Namespace: otherNamespace, Name: "agent-client-cert"},
					[]byte("CERT-OWNED-TRUST-SOURCE"), metadata.Metadata{},
				))
			},
		},
	}
	for _, ts := range trustSources {
		ts.publish(t)
	}

	// the owned trust source is picked up without any other change to the Fleet Server
	test.RetryUntilSuccess(t, func() error {
		got, err := bundle()
		if err != nil {
			return err
		}
		if !strings.Contains(got, "CERT-OWNED-TRUST-SOURCE") {
			return fmt.Errorf("the trust bundle does not contain the published client certificate yet: %q", got)
		}
		return nil
	})

	// the bundle holding the owned trust source, published last, was built from all the other secrets
	got, err := bundle()
	require.NoError(t, err)
	for _, ts := range trustSources {
		t.Run(ts.name, func(t *testing.T) {
			require.Equal(t, ts.wantTrusted, strings.Contains(got, ts.cert))
		})
	}
}
