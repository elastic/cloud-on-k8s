// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package integrations

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	agv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/agent"
	esClient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	agTest "github.com/elastic/cloud-on-k8s/v2/test/e2e/test/agent"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/helper"
)

type chartBuilder struct {
	releaseName           string
	nameSpace             string
	elasticSearch         esv1.Elasticsearch
	chart                 *chart.Chart
	installAction         *action.Install
	uninstallAction       *action.Uninstall
	esClient              esClient.Client
	streamValidationFuncs []agTest.ValidationFunc
	values                map[string]interface{}
}

func (i chartBuilder) InitTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Agent CRDs should exist",
			Test: test.Eventually(func() error {
				crd := &agv1.AgentList{}
				return k.Client.List(context.Background(), crd)
			}),
		},
	}
}

func (i chartBuilder) CreationTestSteps(_ *test.K8sClient) test.StepList {
	return nil
}

func (i chartBuilder) CheckK8sTestSteps(_ *test.K8sClient) test.StepList {
	return nil
}

func (i chartBuilder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Eck Integrations Helm Chart should install",
			Test: func(t *testing.T) {

				esRef := commonv1.ObjectSelector{
					Name:      i.elasticSearch.Name,
					Namespace: i.elasticSearch.Namespace,
				}

				password, err := k.GetElasticPassword(esRef.NamespacedName())
				require.NoError(t, err)

				serviceName := esRef.Name + "-es-http"
				svc, err := k.GetService(esRef.Namespace, serviceName)
				require.NoError(t, err)

				servicePort := svc.Spec.Ports[0]

				i.values["elasticsearch"] = map[string]interface{}{
					"host": fmt.Sprintf("http://%s:%d", svc.Spec.ClusterIP, servicePort.Port),
					"user": "elastic",
					"pass": password,
				}

				i.esClient, err = elasticsearch.NewElasticsearchClient(i.elasticSearch, k)
				require.NoError(t, err)

				helmRelease, err := i.installAction.Run(i.chart, i.values)
				require.NoError(t, err)

				decoder := helper.NewYAMLDecoder()
				renderedObjects, err := decoder.ToObjects(bufio.NewReader(strings.NewReader(helmRelease.Manifest)))
				require.NoError(t, err)

				_ = renderedObjects

				i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
					HasWorkingDataStream(agTest.MetricsType, "kubernetes.container", "default"))
				i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
					HasWorkingDataStream(agTest.MetricsType, "kubernetes.node", "default"))
				i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
					HasWorkingDataStream(agTest.MetricsType, "kubernetes.pod", "default"))
				i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
					HasWorkingDataStream(agTest.MetricsType, "kubernetes.volume", "default"))
				i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
					HasWorkingDataStream(agTest.MetricsType, "kubernetes.system", "default"))
				i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
					HasWorkingDataStream(agTest.MetricsType, "kubernetes.state_container", "default"))
				//i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
				//	HasWorkingDataStream(agTest.MetricsType, "kubernetes.state_cronjob", "default"))
				i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
					HasWorkingDataStream(agTest.MetricsType, "kubernetes.state_daemonset", "default"))
				i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
					HasWorkingDataStream(agTest.MetricsType, "kubernetes.state_deployment", "default"))
				//i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
				//	HasWorkingDataStream(agTest.MetricsType, "kubernetes.state_job", "default"))
				i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
					HasWorkingDataStream(agTest.MetricsType, "kubernetes.state_node", "default"))
				i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
					HasWorkingDataStream(agTest.MetricsType, "kubernetes.state_persistentvolume", "default"))
				i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
					HasWorkingDataStream(agTest.MetricsType, "kubernetes.state_persistentvolumeclaim", "default"))
				i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
					HasWorkingDataStream(agTest.MetricsType, "kubernetes.state_pod", "default"))
				i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
					HasWorkingDataStream(agTest.MetricsType, "kubernetes.state_replicaset", "default"))
				i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
					HasWorkingDataStream(agTest.MetricsType, "kubernetes.state_resourcequota", "default"))
				i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
					HasWorkingDataStream(agTest.MetricsType, "kubernetes.state_service", "default"))
				i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
					HasWorkingDataStream(agTest.MetricsType, "kubernetes.state_statefulset", "default"))
				i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
					HasWorkingDataStream(agTest.MetricsType, "kubernetes.state_storageclass", "default"))
				//i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
				//	HasWorkingDataStream(agTest.LogsType, "kubernetes.audit_logs", "default"))
				i.streamValidationFuncs = append(i.streamValidationFuncs, agTest.
					HasWorkingDataStream(agTest.LogsType, "kubernetes.container_logs", "default"))
			},
		},
		{
			Name: "Agent deployment should be created",
			Test: test.Eventually(func() error {
				var dep appsv1.Deployment
				if err := k.Client.Get(context.Background(), types.NamespacedName{
					Namespace: i.nameSpace,
					Name:      "eck-agent-deployment-agent",
				}, &dep); err != nil {
					return err
				}

				if *dep.Spec.Replicas != 1 {
					return fmt.Errorf("invalid deployment replicas count: expected 1, got %d", *dep.Spec.Replicas)
				}

				return nil
			}),
		},
		{
			Name: "Agent daemonset should be created",
			Test: test.Eventually(func() error {
				var dep appsv1.DaemonSet
				if err := k.Client.Get(context.Background(), types.NamespacedName{
					Namespace: i.nameSpace,
					Name:      "eck-agent-daemonset-agent",
				}, &dep); err != nil {
					return err
				}

				return nil
			}),
		},
		{
			Name: "Agents wait for all pods to be ready",
			Test: test.Eventually(func() error {
				ns := k8sclient.InNamespace(i.nameSpace)
				matchLabels := k8sclient.MatchingLabels(map[string]string{
					commonv1.TypeLabelName: agent.TypeLabelValue,
				})
				listOpts := []k8sclient.ListOption{ns, matchLabels}

				var pods corev1.PodList
				if err := k.Client.List(context.Background(), &pods, listOpts...); err != nil {
					return err
				}

				// pods are running
				for _, pod := range pods.Items {
					if pod.Status.Phase != corev1.PodRunning {
						return fmt.Errorf("pod not running yet")
					}
				}

				// pods are ready
				for _, pod := range pods.Items {
					if !k8s.IsPodReady(pod) {
						return fmt.Errorf("pod not ready yet")
					}
				}

				return nil
			}),
		},
		{
			Name: "ES data should pass validations",
			Test: test.Eventually(func() error {
				for _, validation := range i.streamValidationFuncs {
					if err := validation(i.esClient); err != nil {
						return err
					}
				}

				return nil
			}),
		},
	}
}

func (i chartBuilder) UpgradeTestSteps(_ *test.K8sClient) test.StepList {
	return nil
}

func (i chartBuilder) DeletionTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Eck Integrations Helm Chart should uninstall",
			Test: func(t *testing.T) {
				_, err := i.uninstallAction.Run(i.releaseName)
				require.NoError(t, err)
			},
		},
		{
			Name: "Agent pods should eventually be removed",
			Test: test.Eventually(func() error {
				ns := k8sclient.InNamespace(i.nameSpace)
				matchLabels := k8sclient.MatchingLabels(map[string]string{
					commonv1.TypeLabelName: agent.TypeLabelValue,
				})
				listOpts := []k8sclient.ListOption{ns, matchLabels}

				return k.CheckPodCount(0, listOpts...)
			}),
		},
	}
}

func (i chartBuilder) MutationTestSteps(_ *test.K8sClient) test.StepList {
	return nil
}

func (i chartBuilder) SkipTest() bool {
	return false
}

func newChartBuilder(releaseName string, namespace string, es esv1.Elasticsearch, values map[string]interface{}) (test.Builder, error) {
	settings := cli.New()
	settings.SetNamespace(namespace)
	actionConfig := &action.Configuration{}

	helmChart, err := loader.Load("../../../../deploy/eck-integrations")
	if err != nil {
		return nil, err
	}

	silent := func(format string, v ...interface{}) {}

	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), "", silent); err != nil {
		return nil, err
	}

	installAction := action.NewInstall(actionConfig)
	installAction.Namespace = namespace
	installAction.UseReleaseName = true
	installAction.ReleaseName = releaseName
	installAction.Wait = true
	installAction.Timeout = 2 * time.Minute
	installAction.WaitForJobs = true

	uninstallAction := action.NewUninstall(actionConfig)
	uninstallAction.Wait = true

	return &chartBuilder{
		elasticSearch:   es,
		releaseName:     releaseName,
		chart:           helmChart,
		nameSpace:       namespace,
		installAction:   installAction,
		uninstallAction: uninstallAction,
		values:          values,
	}, nil
}
