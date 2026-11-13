// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build integration

// This test is an external test package (kibana_test) because it registers the
// kb-es association controller, whose package imports pkg/controller/kibana.
package kibana_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/elastic/cloud-on-k8s/v3/pkg/about"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association"
	assocctl "github.com/elastic/cloud-on-k8s/v3/pkg/controller/association/controller"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nsmatch"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/password"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/hints"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/license/trial"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/cryptutil"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/optional"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/rbac"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/retry"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/test"
)

const (
	operatorNamespace = "elastic-system"
	scopeLabelName    = "test-scope"
	scopeLabelValue   = "eck-managed"
)

func TestMain(m *testing.M) {
	test.RunWithK8s(m)
}

// TestCrossNamespaceAssociationAndNamespaceScopeOut runs the Kibana controller, the kb-es
// association controller and the trial license controller against a real API server, with the
// dynamic namespace selector enabled, and verifies that:
//  1. a Kibana referencing an Elasticsearch in another in-scope namespace gets its association
//     established, with the association artifacts created on both sides of the namespace boundary;
//  2. when the Kibana namespace is scoped out (its labels stop matching the selector),
//     reconciliation stops: a deleted controller-owned resource is not recreated;
//  3. when the namespace is scoped back in, the namespace flip watch re-enqueues the Kibana
//     and reconciliation resumes without any event on the Kibana resource itself;
//  4. when the *Elasticsearch* namespace is scoped out instead, the association controller
//     re-enqueues the (still in-scope) Kibana, loses sight of the referenced Elasticsearch
//     through the filtered client and removes the association, so the Kibana's
//     ElasticsearchAssociationStatus is no longer Established; scoping the Elasticsearch
//     namespace back in re-establishes the association.
func TestCrossNamespaceAssociationAndNamespaceScopeOut(t *testing.T) {
	const (
		esNamespace  = "scope-test-es-ns"
		kbNamespace  = "scope-test-kb-ns"
		esName       = "test-es"
		kbName       = "test-kb"
		stackVersion = "8.16.0"
	)
	ctx := context.Background()

	selector := labels.SelectorFromSet(labels.Set{scopeLabelName: scopeLabelValue})
	matcher := nsmatch.NewNamespaceMatcher(selector, operatorNamespace)
	params := operator.Parameters{
		OperatorNamespace: operatorNamespace,
		OperatorInfo:      about.OperatorInfo{BuildInfo: about.BuildInfo{Version: "3.5.0"}},
		IPFamily:          corev1.IPv4Protocol,
		CACertRotation:    certificates.RotationParams{Validity: certificates.DefaultCertValidity, RotateBefore: certificates.DefaultRotateBefore},
		CertRotation:      certificates.RotationParams{Validity: certificates.DefaultCertValidity, RotateBefore: certificates.DefaultRotateBefore},
		NamespaceMatcher:  matcher,
	}

	_, stop := test.StartManager(t, func(mgr manager.Manager, p operator.Parameters) error {
		p.NamespaceMatcher.SetCache(mgr.GetCache())

		hasher, err := cryptutil.NewPasswordHasher(0)
		if err != nil {
			return err
		}
		p.PasswordHasher = hasher

		generator, err := password.NewGenerator(mgr.GetClient(), 24, operatorNamespace)
		if err != nil {
			return err
		}
		p.PasswordGenerator = generator

		// The namespaced reconciler wrapper gates every reconciliation on the real license
		// checker: run the trial controller so that the trial license created below becomes
		// a valid ECK-managed trial and enterprise features are reported as enabled.
		if err := trial.Add(mgr, p); err != nil {
			return err
		}
		if err := assocctl.AddKibanaES(mgr, rbac.NewPermissiveAccessReviewer(), p); err != nil {
			return err
		}
		return kibana.Add(mgr, p)
	}, params, func(o *manager.Options) {
		// mirror the production dynamic namespace-selector wiring (cmd/manager/main.go):
		// the manager client hides out-of-scope resources from the controllers
		o.NewClient = func(config *rest.Config, options client.Options) (client.Client, error) {
			delegate, err := client.New(config, options)
			if err != nil {
				return nil, err
			}
			return nsmatch.NewFilterClient(delegate, matcher), nil
		}
	})
	defer stop()

	// use a direct, unfiltered client for all test operations and assertions: the manager
	// client is now a FilterClient and would hide objects in out-of-scope namespaces,
	// making the negative assertions below vacuous
	c, err := client.New(test.Config, client.Options{})
	require.NoError(t, err)

	// operator namespace is always in scope, no label needed
	require.NoError(t, test.EnsureNamespace(c, operatorNamespace))
	// Elasticsearch and Kibana live in two distinct namespaces, both initially in scope
	for _, ns := range []string{esNamespace, kbNamespace} {
		require.NoError(t, c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:   ns,
			Labels: map[string]string{scopeLabelName: scopeLabelValue},
		}}))
	}

	// start a trial and wait until enterprise features are enabled, otherwise the first Kibana
	// reconciliations would be parked with a long requeue delay
	require.NoError(t, license.CreateTrialLicense(ctx, c, types.NamespacedName{Namespace: operatorNamespace, Name: "eck-trial-license"}))
	checker := license.NewLicenseChecker(c, operatorNamespace)
	retryUntil(t, 60*time.Second, func() error {
		enabled, err := checker.EnterpriseFeaturesEnabled(ctx)
		if err != nil {
			return err
		}
		if !enabled {
			return errors.New("enterprise features not enabled yet")
		}
		return nil
	})

	// Create the referenced Elasticsearch along with the resources its own controller (not
	// running here) would have created and that the association controller depends on:
	// the public HTTP certs secret, the external HTTP service and the status version.
	// The orchestration hint tells the association controller that service account tokens
	// can be used, so no Elasticsearch API call is needed to establish the association.
	hintsAnnotation, err := hints.OrchestrationsHints{ServiceAccounts: optional.NewBool(true)}.AsAnnotation()
	require.NoError(t, err)
	es := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: esName, Namespace: esNamespace, Annotations: hintsAnnotation},
		Spec: esv1.ElasticsearchSpec{
			Version:  stackVersion,
			NodeSets: []esv1.NodeSet{{Name: "default", Count: 1}},
		},
	}
	require.NoError(t, c.Create(ctx, es))
	es.Status.Version = stackVersion
	require.NoError(t, c.Status().Update(ctx, es))

	certsPublic := certificates.PublicCertsSecretRef(esv1.ESNamer, k8s.ExtractNamespacedName(es))
	require.NoError(t, c.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: certsPublic.Name, Namespace: certsPublic.Namespace},
		Data: map[string][]byte{
			certificates.CAFileName:   []byte("fake-ca-cert"),
			certificates.CertFileName: []byte("fake-tls-cert"),
		},
	}))
	require.NoError(t, c.Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: esv1.HTTPService(esName), Namespace: esNamespace},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "https", Port: 9200}}},
	}))

	kb := &kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{Name: kbName, Namespace: kbNamespace},
		Spec: kbv1.KibanaSpec{
			Version:          stackVersion,
			Count:            1,
			ElasticsearchRef: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: esName, Namespace: esNamespace}},
		},
	}
	require.NoError(t, c.Create(ctx, kb))

	// the association must be established across the namespace boundary
	retryUntil(t, 90*time.Second, func() error {
		var current kbv1.Kibana
		if err := c.Get(ctx, k8s.ExtractNamespacedName(kb), &current); err != nil {
			return err
		}
		if s := current.Status.ElasticsearchAssociationStatus; s != commonv1.AssociationEstablished {
			return fmt.Errorf("association status is %q", s)
		}
		return nil
	})

	// the Elasticsearch CA must have been copied into the Kibana namespace
	caCopyKey := types.NamespacedName{
		Namespace: kbNamespace,
		Name:      association.CACertSecretName(kb.EsAssociation(), "kb-es"),
	}
	retryUntil(t, 30*time.Second, func() error {
		var sec corev1.Secret
		return c.Get(ctx, caCopyKey, &sec)
	})

	// the service account token secret must have been created in the Elasticsearch namespace
	esTokenKey := types.NamespacedName{
		Namespace: esNamespace,
		Name:      fmt.Sprintf("%s-%s-kibana-user", kbNamespace, kbName),
	}
	retryUntil(t, 30*time.Second, func() error {
		var sec corev1.Secret
		return c.Get(ctx, esTokenKey, &sec)
	})

	// the Kibana controller must have created the deployment
	deploymentKey := types.NamespacedName{Namespace: kbNamespace, Name: kbv1.KBNamer.Suffix(kbName)}
	retryUntil(t, 90*time.Second, func() error {
		var dep appsv1.Deployment
		return c.Get(ctx, deploymentKey, &dep)
	})

	// scope the Kibana namespace out: its labels no longer match the selector
	setNamespaceLabels(t, c, kbNamespace, nil)
	// let reconciliations that were in flight before the namespace flip settle
	time.Sleep(3 * time.Second)

	// delete the deployment: the deletion event must not trigger a reconciliation
	// since the namespace is out of scope, so the deployment must stay deleted
	var dep appsv1.Deployment
	require.NoError(t, c.Get(ctx, deploymentKey, &dep))
	require.NoError(t, c.Delete(ctx, &dep))
	requireConsistentlyAbsent(t, c, deploymentKey, 5*time.Second)

	// the cross-namespace association artifacts must be left alone on scope-out:
	// out-of-scope resources are no longer reconciled but never cleaned up
	var sec corev1.Secret
	require.NoError(t, c.Get(ctx, esTokenKey, &sec))
	require.NoError(t, c.Get(ctx, caCopyKey, &sec))

	// scope the namespace back in: the namespace flip watch must re-enqueue the Kibana
	// without any event on the Kibana resource itself, and recreate the deployment
	setNamespaceLabels(t, c, kbNamespace, map[string]string{scopeLabelName: scopeLabelValue})
	retryUntil(t, 90*time.Second, func() error {
		var dep appsv1.Deployment
		return c.Get(ctx, deploymentKey, &dep)
	})

	// scope the *Elasticsearch* namespace out: the association controller's namespace flip
	// watch must re-enqueue the Kibana (whose own namespace is still in scope), the filtered
	// client hides the referenced Elasticsearch, and the association must be torn down
	setNamespaceLabels(t, c, esNamespace, nil)
	retryUntil(t, 90*time.Second, func() error {
		var current kbv1.Kibana
		if err := c.Get(ctx, k8s.ExtractNamespacedName(kb), &current); err != nil {
			return err
		}
		if s := current.Status.ElasticsearchAssociationStatus; s != commonv1.AssociationPending {
			return fmt.Errorf("association status is %q, expected %q", s, commonv1.AssociationPending)
		}
		if _, ok := current.Annotations[current.EsAssociation().AssociationConfAnnotationName()]; ok {
			return errors.New("association conf annotation still present")
		}
		return nil
	})

	// scope the Elasticsearch namespace back in: the association must be re-established
	setNamespaceLabels(t, c, esNamespace, map[string]string{scopeLabelName: scopeLabelValue})
	retryUntil(t, 90*time.Second, func() error {
		var current kbv1.Kibana
		if err := c.Get(ctx, k8s.ExtractNamespacedName(kb), &current); err != nil {
			return err
		}
		if s := current.Status.ElasticsearchAssociationStatus; s != commonv1.AssociationEstablished {
			return fmt.Errorf("association status is %q, expected %q", s, commonv1.AssociationEstablished)
		}
		return nil
	})
}

func retryUntil(t *testing.T, timeout time.Duration, f func() error) {
	t.Helper()
	require.NoError(t, retry.UntilSuccess(f, timeout, 500*time.Millisecond))
}

func setNamespaceLabels(t *testing.T, c k8s.Client, name string, lbls map[string]string) {
	t.Helper()
	// retry on conflicts with concurrent namespace updates
	retryUntil(t, 30*time.Second, func() error {
		var ns corev1.Namespace
		if err := c.Get(context.Background(), types.NamespacedName{Name: name}, &ns); err != nil {
			return err
		}
		ns.Labels = lbls
		return c.Update(context.Background(), &ns)
	})
}

// requireConsistentlyAbsent verifies that the object at key stays absent for the whole duration.
func requireConsistentlyAbsent(t *testing.T, c k8s.Client, key types.NamespacedName, duration time.Duration) {
	t.Helper()
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		var dep appsv1.Deployment
		err := c.Get(context.Background(), key, &dep)
		if err == nil {
			t.Fatalf("deployment %s was recreated while its namespace was out of scope", key)
		}
		require.True(t, apierrors.IsNotFound(err), "unexpected error while checking absence: %v", err)
		time.Sleep(250 * time.Millisecond)
	}
}
