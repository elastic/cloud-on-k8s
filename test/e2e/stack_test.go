// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// +build mixed e2e

package e2e

import (
	"context"
	"fmt"
	"testing"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	entv1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/filebeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	beattests "github.com/elastic/cloud-on-k8s/test/e2e/beat"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/enterprisesearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
	"k8s.io/apimachinery/pkg/types"
)

// TestVersionUpgradeOrdering deploys the entire stack, with resources associated together.
// Then, it updates their version, and ensures a strict ordering is respected during the version upgrade.
func TestVersionUpgradeOrdering(t *testing.T) {
	initialVersion := "7.17.0"
	updatedVersion := "8.1.1"

	// upgrading the entire stack can take some time, since we need to account for (in order):
	// - Elasticsearch rolling upgrade
	// - Kibana + Enterprise Search deployments upgrade
	// - APMServer deployment upgrade + Beat daemonset upgrade
	timeout := test.Ctx().TestTimeout * 2

	// Single-node ES clusters cannot be green with APM indices (see https://github.com/elastic/apm-server/issues/414).
	es := elasticsearch.NewBuilder("es").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithVersion(initialVersion).
		WithRestrictedSecurityContext()
	esUpdated := es.WithVersion(updatedVersion)
	esRef := commonv1.ObjectSelector{Namespace: es.Elasticsearch.Namespace, Name: es.Elasticsearch.Name}
	kb := kibana.NewBuilder("kb").
		WithNodeCount(1).
		WithVersion(initialVersion).
		WithElasticsearchRef(esRef).
		WithRestrictedSecurityContext().
		WithAPMIntegration()
	kbUpdated := kb.WithVersion(updatedVersion)
	kbRef := commonv1.ObjectSelector{Namespace: kb.Kibana.Namespace, Name: kb.Kibana.Name}
	apm := apmserver.NewBuilder("apm").
		WithNodeCount(1).
		WithVersion(initialVersion).
		WithElasticsearchRef(esRef).
		WithKibanaRef(kbRef).
		WithRestrictedSecurityContext()
	apmUpdated := apm.WithVersion(updatedVersion)
	ent := enterprisesearch.NewBuilder("ent").
		WithNodeCount(1).
		WithVersion(initialVersion). // pre 8.x doesn't require any config, but we change the version after calling
		WithoutConfig().             // NewBuilder which relies on the version from test.Ctx(), so removing config here
		WithElasticsearchRef(esRef).
		WithRestrictedSecurityContext()
	entUpdated := ent.WithVersion(updatedVersion)
	fb := beat.NewBuilder("fb").
		WithType(filebeat.Type).
		WithRoles(beat.PSPClusterRoleName, beat.AutodiscoverClusterRoleName).
		WithVersion(initialVersion).
		WithElasticsearchRef(esRef).
		WithKibanaRef(kbRef)
	fb = beat.ApplyYamls(t, fb, beattests.E2EFilebeatConfig, beattests.E2EFilebeatPodTemplate)
	fbUpdated := fb.WithVersion(updatedVersion)

	initialBuilders := []test.Builder{es, kb, apm, ent, fb}
	updatedBuilders := []test.Builder{esUpdated, kbUpdated, apmUpdated, entUpdated, fbUpdated}

	versionUpgrade := func(k *test.K8sClient) test.StepList {
		steps := test.StepList{}
		// upgrade the version of all resources
		for _, b := range updatedBuilders {
			steps = steps.WithSteps(b.UpgradeTestSteps(k))
		}
		// wait until they're all upgraded, while ensuring the upgrade order is respected
		return steps.WithStep(test.Step{
			Name: "Check all resources are eventually upgraded in the right order",
			Test: test.UntilSuccess(func() error {
				// retrieve the version from the status of all resources
				stackVersions := StackResourceVersions{
					Elasticsearch:    ref(k8s.ExtractNamespacedName(&es.Elasticsearch)),
					Kibana:           ref(k8s.ExtractNamespacedName(&kb.Kibana)),
					ApmServer:        ref(k8s.ExtractNamespacedName(&apm.ApmServer)),
					EnterpriseSearch: ref(k8s.ExtractNamespacedName(&ent.EnterpriseSearch)),
					Beat:             ref(k8s.ExtractNamespacedName(&fb.Beat)),
				}
				err := stackVersions.Retrieve(k.Client)
				// check the retrieved versions first (before returning on err)
				t.Log(stackVersions)
				if !stackVersions.IsValid() {
					t.Fatal("invalid stack versions upgrade order", stackVersions)
				}
				if err != nil {
					return err
				}
				if !stackVersions.AllSetTo(updatedVersion) {
					return fmt.Errorf("some versions are still not updated: %+v", stackVersions)
				}
				return nil
			}, timeout),
		})
	}

	test.Sequence(nil, versionUpgrade, initialBuilders...).RunSequential(t)
}

type StackResourceVersions struct {
	Elasticsearch    refVersion
	Kibana           refVersion
	ApmServer        refVersion
	EnterpriseSearch refVersion
	Beat             refVersion
}

func (s StackResourceVersions) IsValid() bool {
	// ES >= Kibana >= (Beats, APM)
	return s.Elasticsearch.GTE(s.Kibana) &&
		s.Kibana.GTE(s.Beat) &&
		s.Kibana.GTE(s.ApmServer) &&
		// ES >= EnterpriseSearch
		s.Elasticsearch.GTE(s.EnterpriseSearch)
}

func (s StackResourceVersions) AllSetTo(version string) bool {
	for _, ref := range []refVersion{s.Elasticsearch, s.Kibana, s.ApmServer, s.EnterpriseSearch, s.Beat} {
		if ref.version != version {
			return false
		}
	}
	return true
}

func (s *StackResourceVersions) Retrieve(client k8s.Client) error {
	calls := []func(c k8s.Client) error{s.retrieveBeat, s.retrieveApmServer, s.retrieveKibana, s.retrieveEnterpriseSearch, s.retrieveElasticsearch}
	// grab at least one error if multiple occur
	var callsErr error
	for _, f := range calls {
		if err := f(client); err != nil {
			callsErr = err
		}
	}
	return callsErr
}

type refVersion struct {
	ref     types.NamespacedName
	version string
}

func (r refVersion) GTE(r2 refVersion) bool {
	if r.version == "" || r2.version == "" {
		// empty version, consider it's ok
		return true
	}
	rVersion := version.MustParse(r.version)
	r2Version := version.MustParse(r2.version)
	return rVersion.GTE(r2Version)
}

func ref(ref types.NamespacedName) refVersion {
	return refVersion{ref: ref}
}

func (s *StackResourceVersions) retrieveElasticsearch(c k8s.Client) error {
	var es esv1.Elasticsearch
	if err := c.Get(context.Background(), s.Elasticsearch.ref, &es); err != nil {
		return err
	}
	s.Elasticsearch.version = es.Status.Version
	return nil
}

func (s *StackResourceVersions) retrieveKibana(c k8s.Client) error {
	var kb kbv1.Kibana
	if err := c.Get(context.Background(), s.Kibana.ref, &kb); err != nil {
		return err
	}
	s.Kibana.version = kb.Status.Version
	return nil
}

func (s *StackResourceVersions) retrieveApmServer(c k8s.Client) error {
	var as apmv1.ApmServer
	if err := c.Get(context.Background(), s.ApmServer.ref, &as); err != nil {
		return err
	}
	s.ApmServer.version = as.Status.Version
	return nil
}

func (s *StackResourceVersions) retrieveEnterpriseSearch(c k8s.Client) error {
	var ent entv1.EnterpriseSearch
	if err := c.Get(context.Background(), s.EnterpriseSearch.ref, &ent); err != nil {
		return err
	}
	s.EnterpriseSearch.version = ent.Status.Version
	return nil
}

func (s *StackResourceVersions) retrieveBeat(c k8s.Client) error {
	var beat beatv1beta1.Beat
	if err := c.Get(context.Background(), s.Beat.ref, &beat); err != nil {
		return err
	}
	s.Beat.version = beat.Status.Version
	return nil
}
