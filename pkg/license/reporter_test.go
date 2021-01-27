// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	commonlicense "github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const operatorNs = "test-system"

func TestGet(t *testing.T) {
	t.Run("elasticsearch_defaults", func(t *testing.T) {
		es := esv1.Elasticsearch{
			Spec: esv1.ElasticsearchSpec{
				NodeSets: []esv1.NodeSet{{
					Count: 10,
				}},
			},
		}
		have, err := NewResourceReporter(k8s.NewFakeClient(&es), operatorNs).Get()
		require.NoError(t, err)

		want := LicensingInfo{
			TotalManagedMemory:      21.47,
			EnterpriseResourceUnits: 1,
			EckLicenseLevel:         "basic",
		}

		assertEqual(t, want, have)
	})

	t.Run("elasticsearch_with_resource_limits", func(t *testing.T) {
		es := esv1.Elasticsearch{
			Spec: esv1.ElasticsearchSpec{
				NodeSets: []esv1.NodeSet{{
					Count: 100,
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: esv1.ElasticsearchContainerName,
									Resources: corev1.ResourceRequirements{
										Limits: map[corev1.ResourceName]resource.Quantity{
											corev1.ResourceMemory: resource.MustParse("6Gi"),
										},
									},
								},
							},
						},
					},
				}},
			},
		}
		have, err := NewResourceReporter(k8s.NewFakeClient(&es), operatorNs).Get()
		require.NoError(t, err)

		want := LicensingInfo{
			TotalManagedMemory:      644.245,
			EnterpriseResourceUnits: 11,
			EckLicenseLevel:         "basic",
		}

		assertEqual(t, want, have)
	})

	t.Run("elasticsearch_with_heap_settings", func(t *testing.T) {
		es := esv1.Elasticsearch{
			Spec: esv1.ElasticsearchSpec{
				NodeSets: []esv1.NodeSet{{
					Count: 10,
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: esv1.ElasticsearchContainerName,
									Env: []corev1.EnvVar{{
										Name: "ES_JAVA_OPTS", Value: "-Xms8G -Xmx8G",
									}},
								},
							},
						},
					},
				}},
			},
		}

		have, err := NewResourceReporter(k8s.NewFakeClient(&es), operatorNs).Get()
		require.NoError(t, err)

		want := LicensingInfo{
			TotalManagedMemory:      171.801,
			EnterpriseResourceUnits: 3,
			EckLicenseLevel:         "basic",
		}

		assertEqual(t, want, have)
	})

	t.Run("kibana_defaults", func(t *testing.T) {
		kb := kbv1.Kibana{
			Spec: kbv1.KibanaSpec{
				Count: 100,
			},
		}

		have, err := NewResourceReporter(k8s.NewFakeClient(&kb), operatorNs).Get()
		require.NoError(t, err)

		want := LicensingInfo{
			TotalManagedMemory:      107.374,
			EnterpriseResourceUnits: 2,
			EckLicenseLevel:         "basic",
		}

		assertEqual(t, want, have)
	})

	t.Run("kibana_with_resource_limits", func(t *testing.T) {
		kb := kbv1.Kibana{
			Spec: kbv1.KibanaSpec{
				Count: 100,
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: kbv1.KibanaContainerName,
								Resources: corev1.ResourceRequirements{
									Limits: map[corev1.ResourceName]resource.Quantity{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
									},
								},
							},
						},
					},
				},
			},
		}

		have, err := NewResourceReporter(k8s.NewFakeClient(&kb), operatorNs).Get()
		require.NoError(t, err)
		want := LicensingInfo{
			TotalManagedMemory:      214.754,
			EnterpriseResourceUnits: 4,
			EckLicenseLevel:         "basic",
		}

		assertEqual(t, want, have)
	})

	t.Run("kibana_with_node_opts", func(t *testing.T) {
		kb := kbv1.Kibana{
			Spec: kbv1.KibanaSpec{
				Count: 100,
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: kbv1.KibanaContainerName,
								Env: []corev1.EnvVar{{
									Name: kibana.EnvNodeOpts, Value: "--max-old-space-size=2048",
								}},
							},
						},
					},
				},
			},
		}
		have, err := NewResourceReporter(k8s.NewFakeClient(&kb), operatorNs).Get()
		require.NoError(t, err)
		want := LicensingInfo{
			TotalManagedMemory:      204.804,
			EnterpriseResourceUnits: 4,
			EckLicenseLevel:         "basic",
		}

		assertEqual(t, want, have)
	})
}

func assertEqual(t *testing.T, want, have LicensingInfo) {
	t.Helper()

	wantMap := want.toMap()
	delete(wantMap, "timestamp")

	haveMap := have.toMap()
	delete(haveMap, "timestamp")

	require.Equal(t, wantMap, haveMap)
}

func Test_Start(t *testing.T) {
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name: "es-test",
		},
		Spec: esv1.ElasticsearchSpec{NodeSets: []esv1.NodeSet{{Count: 40}}}}
	kb := kbv1.Kibana{Spec: kbv1.KibanaSpec{Count: 2}}
	apm := apmv1.ApmServer{Spec: apmv1.ApmServerSpec{Count: 2}}
	k8sClient := k8s.NewFakeClient(&es, &kb, &apm)
	refreshPeriod := 1 * time.Second
	waitFor := 10 * refreshPeriod
	tick := refreshPeriod / 2

	// start the resource reporter
	go NewResourceReporter(k8sClient, operatorNs).Start(refreshPeriod)

	// check that the licensing config map exists
	assert.Eventually(t, func() bool {
		var cm corev1.ConfigMap
		err := k8sClient.Get(context.Background(), types.NamespacedName{
			Namespace: operatorNs,
			Name:      LicensingCfgMapName,
		}, &cm)
		if err != nil {
			return false
		}
		return cm.Data["timestamp"] != "" &&
			cm.Data["eck_license_level"] == defaultOperatorLicenseLevel &&
			cm.Data["enterprise_resource_units"] == "2" &&
			cm.Data["total_managed_memory"] == "89.12GB"
	}, waitFor, tick)

	// increase the Elasticsearch nodes count
	es.Spec.NodeSets[0].Count = 80
	err := k8sClient.Update(context.Background(), &es)
	assert.NoError(t, err)

	// check that the licensing config map has been updated
	assert.Eventually(t, func() bool {
		var cm corev1.ConfigMap
		err := k8sClient.Get(context.Background(), types.NamespacedName{
			Namespace: operatorNs,
			Name:      LicensingCfgMapName,
		}, &cm)
		if err != nil {
			return false
		}
		return cm.Data["timestamp"] != "" &&
			cm.Data["eck_license_level"] == defaultOperatorLicenseLevel &&
			cm.Data["enterprise_resource_units"] == "3" &&
			cm.Data["total_managed_memory"] == "175.02GB"
	}, waitFor, tick)

	startTrial(t, k8sClient)
	// check that the license level has been updated
	assert.Eventually(t, func() bool {
		var cm corev1.ConfigMap
		err := k8sClient.Get(context.Background(), types.NamespacedName{
			Namespace: operatorNs,
			Name:      LicensingCfgMapName,
		}, &cm)
		if err != nil {
			return false
		}
		return cm.Data["timestamp"] != "" &&
			cm.Data["eck_license_level"] == string(commonlicense.LicenseTypeEnterpriseTrial) &&
			cm.Data["enterprise_resource_units"] == "3" &&
			cm.Data["total_managed_memory"] == "175.02GB"
	}, waitFor, tick)

}

func startTrial(t *testing.T, k8sClient client.Client) {
	// start a trial
	trialState, err := commonlicense.NewTrialState()
	require.NoError(t, err)
	wrappedClient := k8sClient
	licenseNSN := types.NamespacedName{
		Namespace: operatorNs,
		Name:      "eck-trial",
	}
	// simulate user kicking off the trial activation
	require.NoError(t, commonlicense.CreateTrialLicense(wrappedClient, licenseNSN))
	// fetch user created license
	licenseSecret, license, err := commonlicense.TrialLicense(wrappedClient, licenseNSN)
	require.NoError(t, err)
	// fill in and sign
	require.NoError(t, trialState.InitTrialLicense(&license))
	status, err := commonlicense.ExpectedTrialStatus(operatorNs, licenseNSN, trialState)
	require.NoError(t, err)
	// persist status
	require.NoError(t, wrappedClient.Create(context.Background(), &status))
	// persist updated license
	require.NoError(t, commonlicense.UpdateEnterpriseLicense(wrappedClient, licenseSecret, license))
}
