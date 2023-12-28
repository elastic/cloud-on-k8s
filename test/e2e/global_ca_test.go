// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build mixed || e2e

package e2e

import (
	"context"
	"crypto/x509/pkix"
	"fmt"
	"testing"
	"time"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	e2e_agent "github.com/elastic/cloud-on-k8s/v2/test/e2e/agent"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	elasticagent "github.com/elastic/cloud-on-k8s/v2/test/e2e/test/agent"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/enterprisesearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/kibana"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/logstash"
)

func TestGlobalCA(t *testing.T) {

	// Skip if it is the resilience pipeline because the ChaosJob can prevent
	// assert_operator_has_been_restarted_once_more to pass when it deletes an operator Pod
	// exactly on restart.
	if test.Ctx().Pipeline == "e2e/resilience" {
		t.Skip()
	}

	k := test.NewK8sClientOrFatal()
	name := "global-ca"
	es := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithGlobalCA(true)
	kb := kibana.NewBuilder(name).
		WithNodeCount(1).
		WithElasticsearchRef(es.Ref()).
		WithGlobalCA(true)
	ent := enterprisesearch.NewBuilder(name).
		WithNodeCount(1).
		WithElasticsearchRef(es.Ref()).
		WithRestrictedSecurityContext().
		WithGlobalCA(true)
	testPod := beat.NewPodBuilder(name)
	agent := elasticagent.NewBuilder(name).
		WithElasticsearchRefs(elasticagent.ToOutput(es.Ref(), "default")).
		WithDefaultESValidation(elasticagent.HasWorkingDataStream(elasticagent.LogsType, "elastic_agent", "default"))
	agent = elasticagent.ApplyYamls(t, agent, e2e_agent.E2EAgentSystemIntegrationConfig, e2e_agent.E2EAgentSystemIntegrationPodTemplate)
	ls := logstash.NewBuilder(name).
		WithNodeCount(1).
		WithElasticsearchRefs(
			logstashv1alpha1.ElasticsearchCluster{
				ObjectSelector: es.Ref(),
				ClusterName:    "es",
			}).
		WithGlobalCA(true)

	// create a self-signed CA for testing purposes
	duration := 48 * time.Hour
	ca, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{
		Subject: pkix.Name{
			CommonName:         test.Ctx().TestRun,
			OrganizationalUnit: []string{"eck-e2e"},
		},
		ExpireIn: &duration,
	})
	require.NoError(t, err)
	// update the pre-created secret mounted into the operator with the CA
	secret, err := operatorSecretForCA(ca)
	require.NoError(t, err)
	_, err = reconciler.ReconcileSecret(context.Background(), k.Client, secret, nil)
	require.NoError(t, err)
	// reconfigure the operator to use the CA
	require.NoError(t, addToOperatorConfig(k.Client, "ca", "/tmp/ca-certs"))

	// keep track of operator restarts
	var restartCount int32

	// then on update re-configure the operator to go back to self-signed certificates and verify that applications are
	// reconfigured. Because this is not a real resource update we need to do trickery with the builders to avoid steps that
	// check for update rollout (e.g. observed generation or hash changes)
	kbUpd := kb.DeepCopy().WithGlobalCA(false)
	entUpd := ent.DeepCopy().WithGlobalCA(false)
	lsUpd := ls.DeepCopy().WithGlobalCA(false)

	esUpd := test.WrappedBuilder{
		BuildingThis: es.DeepCopy().WithGlobalCA(false),
		PreMutationSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{
				{
					Name: "retrieve current operator restart count",
					Test: func(t *testing.T) {
						restartCount, err = operatorRestartCount(k)
						require.NoError(t, err)
					},
				},
				{
					Name: "reset operator to use self-signed certificates per resource",
					Test: func(t *testing.T) {
						require.NoError(t, removeFromOperatorConfig(k.Client, "ca"))
					},
				},
				{
					Name: "assert operator has been restarted once more",
					Test: test.Eventually(func() error {
						newCount, err := operatorRestartCount(k)
						if err != nil {
							return err
						}
						if newCount <= restartCount {
							return fmt.Errorf("operator restart count was %d but expected at least %d", newCount, restartCount+1)
						}
						return nil

					}),
				},
			}
		},
		PostMutationSteps: func(k *test.K8sClient) test.StepList {
			// add the other builder checks here because we are not really mutating the resources we just want to check
			// that the CA change gets picked up and secrets are created for example
			return kbUpd.CheckK8sTestSteps(k).WithSteps(
				entUpd.CheckK8sTestSteps(k).WithSteps(
					lsUpd.CheckK8sTestSteps(k),
				),
			)
		},
	}

	test.RunMutations(t, []test.Builder{es, kb, ent, agent, ls, testPod}, []test.Builder{esUpd})
}

func removeFromOperatorConfig(k k8s.Client, key string) error {
	return updateOperatorConfig(k, func(cfg map[string]interface{}) {
		delete(cfg, key)
	})
}

func addToOperatorConfig(k k8s.Client, key, value string) error {
	return updateOperatorConfig(k, func(cfg map[string]interface{}) {
		cfg[key] = value
	})
}

func updateOperatorConfig(k k8s.Client, f func(map[string]interface{})) error {
	var cm corev1.ConfigMap
	if err := k.Get(context.Background(),
		types.NamespacedName{Name: fmt.Sprintf("%s-operator", test.Ctx().TestRun), Namespace: test.Ctx().Operator.Namespace},
		&cm,
	); err != nil {
		return err
	}
	raw := cm.Data["eck.yaml"]
	config := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(raw), &config); err != nil {
		return err
	}
	f(config)
	bytes, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	cm.Data["eck.yaml"] = string(bytes)
	return k.Update(context.Background(), &cm)
}

func operatorRestartCount(k *test.K8sClient) (int32, error) {
	pods, err := k.GetPods(test.OperatorPodListOptions(test.Ctx().Operator.Namespace)...)
	if err != nil {
		return 0, err
	}
	for _, p := range pods {
		for _, c := range p.Status.ContainerStatuses {
			if c.Name == "manager" {
				return c.RestartCount, nil
			}
		}
	}
	return 0, fmt.Errorf("could not find operator container")
}

func operatorSecretForCA(
	ca *certificates.CA,
) (corev1.Secret, error) {
	privateKeyData, err := certificates.EncodePEMPrivateKey(ca.PrivateKey)
	if err != nil {
		return corev1.Secret{}, err
	}
	return corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Namespace: test.Ctx().Operator.Namespace,
			Name:      fmt.Sprintf("eck-ca-%s", test.Ctx().TestRun),
		},
		Data: map[string][]byte{
			certificates.CertFileName: certificates.EncodePEMCert(ca.Cert.Raw),
			certificates.KeyFileName:  privateKeyData,
		},
	}, nil
}
