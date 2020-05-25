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
	essettings "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const operatorNs = "test-system"

func Test_Get(t *testing.T) {
	es := esv1.Elasticsearch{
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{{
				Count: 10,
			}},
		},
	}
	licensingInfo, err := NewResourceReporter(k8s.FakeClient(&es), operatorNs).Get()
	assert.NoError(t, err)
	assert.Equal(t, "21.47GB", licensingInfo.TotalManagedMemory)
	assert.Equal(t, "1", licensingInfo.EnterpriseResourceUnits)

	es = esv1.Elasticsearch{
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
	licensingInfo, err = NewResourceReporter(k8s.FakeClient(&es), operatorNs).Get()
	assert.NoError(t, err)
	assert.Equal(t, "644.25GB", licensingInfo.TotalManagedMemory)
	assert.Equal(t, "11", licensingInfo.EnterpriseResourceUnits)

	es = esv1.Elasticsearch{
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
	licensingInfo, err = NewResourceReporter(k8s.FakeClient(&es), operatorNs).Get()
	assert.NoError(t, err)
	assert.Equal(t, "171.80GB", licensingInfo.TotalManagedMemory)
	assert.Equal(t, "3", licensingInfo.EnterpriseResourceUnits)

	es = esv1.Elasticsearch{
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{{
				Count: 10,
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: esv1.ElasticsearchContainerName,
								Env: []corev1.EnvVar{{
									Name: essettings.EnvEsJavaOpts, Value: "-Xms8G -Xmx8G",
								}},
							},
						},
					},
				},
			}},
		},
	}
	licensingInfo, err = NewResourceReporter(k8s.FakeClient(&es), operatorNs).Get()
	assert.NoError(t, err)
	assert.Equal(t, "171.80GB", licensingInfo.TotalManagedMemory)
	assert.Equal(t, "3", licensingInfo.EnterpriseResourceUnits)

	kb := kbv1.Kibana{
		Spec: kbv1.KibanaSpec{
			Count: 100,
		},
	}
	licensingInfo, err = NewResourceReporter(k8s.FakeClient(&kb), operatorNs).Get()
	assert.NoError(t, err)
	assert.Equal(t, "107.37GB", licensingInfo.TotalManagedMemory)
	assert.Equal(t, "2", licensingInfo.EnterpriseResourceUnits)

	kb = kbv1.Kibana{
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
	licensingInfo, err = NewResourceReporter(k8s.FakeClient(&kb), operatorNs).Get()
	assert.NoError(t, err)
	assert.Equal(t, "214.75GB", licensingInfo.TotalManagedMemory)
	assert.Equal(t, "4", licensingInfo.EnterpriseResourceUnits)

	kb = kbv1.Kibana{
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
	licensingInfo, err = NewResourceReporter(k8s.FakeClient(&kb), operatorNs).Get()
	assert.NoError(t, err)
	assert.Equal(t, "204.80GB", licensingInfo.TotalManagedMemory)
	assert.Equal(t, "4", licensingInfo.EnterpriseResourceUnits)
}

func Test_Start(t *testing.T) {
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name: "es-test",
		},
		Spec: esv1.ElasticsearchSpec{NodeSets: []esv1.NodeSet{{Count: 40}}}}
	kb := kbv1.Kibana{Spec: kbv1.KibanaSpec{Count: 2}}
	apm := apmv1.ApmServer{Spec: apmv1.ApmServerSpec{Count: 2}}
	k8sClient := k8s.FakeClient(&es, &kb, &apm)
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
			Name:      licensingCfgMapName,
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
			Name:      licensingCfgMapName,
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
			Name:      licensingCfgMapName,
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
	wrappedClient := k8s.WrapClient(k8sClient)
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
	require.NoError(t, wrappedClient.Create(&status))
	// persist updated license
	require.NoError(t, commonlicense.UpdateEnterpriseLicense(wrappedClient, licenseSecret, license))
}
