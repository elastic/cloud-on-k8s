// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/bootstrap"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/certificates/transport"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

const (
	// RollingUpgradeTimeout is used for checking a rolling upgrade is complete.
	// Most tests require less than 5 minutes for all Pods to be running and ready,
	// but it occasionally takes longer for various reasons (long Pod creation time, long volume binding, etc.).
	// We use a longer timeout here to not be impacted too much by those external factors, and only fail
	// if things seem to be stuck.
	RollingUpgradeTimeout = 30 * time.Minute
)

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		CheckHTTPCertificateAuthority(b, k),
		CheckTransportCertificateAuthority(b, k),
		CheckExpectedPodsEventuallyReady(b, k),
		CheckESVersion(b, k),
		CheckServices(b, k),
		CheckServicesEndpoints(b, k),
		CheckSecrets(b, k),
		CheckPodCertificates(b, k),
		CheckClusterHealth(b, k),
		CheckESPassword(b, k),
		CheckESDataVolumeType(b.Elasticsearch, k),
		CheckClusterUUIDAnnotation(b.Elasticsearch, k),
	}
}

// CheckHTTPCertificateAuthority checks that the CA is fully setup (CA cert + private key)
func CheckHTTPCertificateAuthority(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ES HTTP certificate authority should be set and deployed",
		Test: test.Eventually(func() error {
			// Check that the HTTP CA may be loaded
			_, err := k.GetCA(b.Elasticsearch.Namespace, b.Elasticsearch.Name, certificates.HTTPCAType)
			if err != nil {
				return err
			}

			return nil
		}),
	}
}

// CheckTransportCertificateAuthority checks that the CA is fully setup (CA cert + private key)
func CheckTransportCertificateAuthority(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ES transport certificate authority should be set and deployed",
		Test: test.Eventually(func() error {
			// Check that the Transport CA may be loaded
			_, err := k.GetCA(b.Elasticsearch.Namespace, b.Elasticsearch.Name, certificates.TransportCAType)
			if err != nil {
				return err
			}
			return nil
		}),
		Skip: func() bool {
			return b.Elasticsearch.Spec.Transport.TLS.UserDefinedCA()
		},
	}
}

// CheckSecrets checks that expected secrets have been created.
func CheckSecrets(b Builder, k *test.K8sClient) test.Step {
	return test.CheckSecretsContent(k, b.Elasticsearch.Namespace, func() []test.ExpectedSecret {
		esName := b.Elasticsearch.Name
		// hardcode all secret names and keys to catch any breaking change
		expected := []test.ExpectedSecret{
			{
				Name: esName + "-es-elastic-user",
				Keys: []string{"elastic"},
				Labels: map[string]string{
					"common.k8s.elastic.co/type":                "elasticsearch",
					"eck.k8s.elastic.co/credentials":            "true",
					"elasticsearch.k8s.elastic.co/cluster-name": esName,
				},
			},
			{
				Name: esName + "-es-http-ca-internal",
				Keys: []string{"tls.crt", "tls.key"},
				Labels: map[string]string{
					"common.k8s.elastic.co/type":                "elasticsearch",
					"elasticsearch.k8s.elastic.co/cluster-name": esName,
				},
			},
			{
				Name: esName + "-es-http-certs-internal",
				Keys: []string{"tls.crt", "tls.key", "ca.crt"},
				Labels: map[string]string{
					"common.k8s.elastic.co/type":                "elasticsearch",
					"elasticsearch.k8s.elastic.co/cluster-name": esName,
				},
			},
			{
				Name: esName + "-es-http-certs-public",
				Keys: []string{"tls.crt", "ca.crt"},
				Labels: map[string]string{
					"common.k8s.elastic.co/type":                "elasticsearch",
					"elasticsearch.k8s.elastic.co/cluster-name": esName,
				},
			},
			{
				Name: esName + "-es-internal-users",
				Keys: []string{"elastic-internal", "elastic-internal-monitoring", "elastic-internal-probe"},
				Labels: map[string]string{
					"common.k8s.elastic.co/type":                "elasticsearch",
					"eck.k8s.elastic.co/credentials":            "true",
					"elasticsearch.k8s.elastic.co/cluster-name": esName,
				},
			},
			{
				Name: esName + "-es-remote-ca",
				Keys: []string{"ca.crt"},
				Labels: map[string]string{
					"elasticsearch.k8s.elastic.co/cluster-name": esName,
				},
			},
			{
				Name: esName + "-es-transport-certs-public",
				Keys: []string{"ca.crt"},
				Labels: map[string]string{
					"common.k8s.elastic.co/type":                "elasticsearch",
					"elasticsearch.k8s.elastic.co/cluster-name": esName,
				},
			},
			{
				Name: esName + "-es-xpack-file-realm",
				Keys: []string{"users", "users_roles", "roles.yml", "service_tokens"},
				Labels: map[string]string{
					"common.k8s.elastic.co/type":                "elasticsearch",
					"elasticsearch.k8s.elastic.co/cluster-name": esName,
				},
			},
			// esName + "-es-transport-certificates" is handled in CheckPodCertificates
		}
		// check internal TLS CA if no user provided CA is spec'ed
		if !b.Elasticsearch.Spec.Transport.TLS.UserDefinedCA() {
			expected = append(expected, test.ExpectedSecret{
				Name: esName + "-es-transport-ca-internal",
				Keys: []string{"tls.crt", "tls.key"},
				Labels: map[string]string{
					"common.k8s.elastic.co/type":                "elasticsearch",
					"elasticsearch.k8s.elastic.co/cluster-name": esName,
				},
			})
		}

		for _, nodeSet := range b.Elasticsearch.Spec.NodeSets {
			expected = append(expected, test.ExpectedSecret{
				Name: esName + "-es-" + nodeSet.Name + "-es-config",
				Keys: []string{"elasticsearch.yml"},
				Labels: map[string]string{
					"common.k8s.elastic.co/type":                    "elasticsearch",
					"elasticsearch.k8s.elastic.co/cluster-name":     esName,
					"elasticsearch.k8s.elastic.co/statefulset-name": esName + "-es-" + nodeSet.Name,
				},
			})
		}
		return expected
	})
}

// CheckPodCertificates checks that all pods have a private key and signed certificate
func CheckPodCertificates(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ES pods should eventually have a certificate",
		Test: test.Eventually(func() error {
			pods, err := k.GetPods(test.ESPodListOptions(b.Elasticsearch.Namespace, b.Elasticsearch.Name)...)
			if err != nil {
				return err
			}
			for _, pod := range pods {
				statefulSet, exist := pod.Labels[label.StatefulSetNameLabelName]
				if !exist {
					return fmt.Errorf("label %s not found on pod %s/%s", label.StatefulSetNameLabelName, pod.Namespace, pod.Name)
				}
				_, _, err := getTransportCert(k, b.Elasticsearch.Namespace, b.Elasticsearch.Name, statefulSet, pod.Name)
				if err != nil {
					return err
				}
			}
			return nil
		}),
	}
}

// getTransportCert retrieves the certificate of the CA and the transport certificate
func getTransportCert(k *test.K8sClient, esNamespace, esName, statefulSetName, podName string) (caCert, transportCert []*x509.Certificate, err error) {
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: esNamespace,
		Name:      statefulSetName + "-es-transport-certs",
	}
	if err = k.Client.Get(context.Background(), key, &secret); err != nil {
		return nil, nil, err
	}
	caCertBytes, exists := secret.Data[certificates.CAFileName]
	if !exists || len(caCertBytes) == 0 {
		return nil, nil, fmt.Errorf("no value found for secret %s", certificates.CAFileName)
	}
	caCert, err = certificates.ParsePEMCerts(caCertBytes)
	if err != nil {
		return nil, nil, err
	}
	transportCertBytes, exists := secret.Data[transport.PodCertFileName(podName)]
	if !exists || len(transportCertBytes) == 0 {
		return nil, nil, fmt.Errorf("no value found for secret %s", transport.PodCertFileName(podName))
	}
	transportCert, err = certificates.ParsePEMCerts(transportCertBytes)
	if err != nil {
		return nil, nil, err
	}
	return
}

// CheckESPodsRunning checks that all ES pods for the given ES are running
func CheckESPodsPending(b Builder, k *test.K8sClient) test.Step {
	return checkESPodsPhase(b, k, corev1.PodPending)
}

func checkESPodsPhase(b Builder, k *test.K8sClient, phase corev1.PodPhase) test.Step {
	return CheckPodsCondition(b,
		k,
		fmt.Sprintf("Pods should eventually be %s", phase),
		func(p corev1.Pod) error {
			if p.Status.Phase != phase {
				return fmt.Errorf("pod not %s", phase)
			}
			return nil
		},
	)
}

func CheckPodsCondition(b Builder, k *test.K8sClient, name string, condition func(p corev1.Pod) error) test.Step {
	return test.Step{
		Name: name,
		Test: test.Eventually(func() error {
			pods, err := k.GetPods(test.ESPodListOptions(b.Elasticsearch.Namespace, b.Elasticsearch.Name)...)
			if err != nil {
				return err
			}
			if int32(len(pods)) != b.Elasticsearch.Spec.NodeCount() {
				return fmt.Errorf("expected %d pods, got %d", len(pods), b.Elasticsearch.Spec.NodeCount())
			}
			return test.OnAllPods(pods, condition)
		}),
	}
}

// CheckESVersion checks that the running ES version is the expected one
func CheckESVersion(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ES version should be the expected one",
		Test: test.Eventually(func() error {
			pods, err := k.GetPods(test.ESPodListOptions(b.Elasticsearch.Namespace, b.Elasticsearch.Name)...)
			if err != nil {
				return err
			}
			expectedEs := b.GetExpectedElasticsearch()
			// check number of pods
			if len(pods) != int(expectedEs.Spec.NodeCount()) {
				return fmt.Errorf("expected %d pods, got %d", b.Elasticsearch.Spec.NodeCount(), len(pods))
			}
			// check ES version label
			for _, p := range pods {
				version := p.Labels[label.VersionLabelName]
				if version != b.Elasticsearch.Spec.Version {
					return fmt.Errorf("version %s does not match expected version %s", version, b.Elasticsearch.Spec.Version)
				}
			}
			// check reported version in the resource status
			var actualEs esv1.Elasticsearch
			if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&expectedEs), &actualEs); err != nil {
				return err
			}
			if actualEs.Status.Version != expectedEs.Spec.Version {
				return fmt.Errorf("version %s in status does not match expected version %s", actualEs.Status.Version, b.Elasticsearch.Spec.Version)
			}
			return nil
		}),
	}
}

// CheckClusterHealth checks that the given ES status reports a green ES health
func CheckClusterHealth(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ES cluster health should eventually be green",
		Test: test.Eventually(func() error {
			return clusterHealthIs(b, k, esv1.ElasticsearchGreenHealth)
		}),
	}
}

// CheckSpecificClusterHealth checks that the given ES status reports a given ES health
func CheckSpecificClusterHealth(b Builder, k *test.K8sClient, health esv1.ElasticsearchHealth) test.Step {
	return test.Step{
		Name: fmt.Sprintf("ES cluster health should eventually be %s", string(health)),
		Test: test.Eventually(func() error {
			return clusterHealthIs(b, k, health)
		}),
	}
}

func clusterHealthIs(b Builder, k *test.K8sClient, health esv1.ElasticsearchHealth) error {
	var es esv1.Elasticsearch
	err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Elasticsearch), &es)
	if err != nil {
		return err
	}
	if es.Status.Health != health {
		return fmt.Errorf("health is %s", es.Status.Health)
	}
	return nil
}

// CheckServices checks that all ES services are created and external IP is provisioned for all LB services
func CheckServices(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ES services should be created",
		Test: test.Eventually(func() error {
			for _, s := range []string{
				// we intentionally hardcode the names here to catch any accidental breaking change
				b.Elasticsearch.Name + "-es-http",
				b.Elasticsearch.Name + "-es-transport",
			} {
				svc, err := k.GetService(b.Elasticsearch.Namespace, s)
				if err != nil {
					return err
				}
				if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
					if len(svc.Status.LoadBalancer.Ingress) == 0 {
						return fmt.Errorf("load balancer for %s not ready yet", svc.Name)
					}
				}
			}
			return nil
		}),
	}
}

// CheckServicesEndpoints checks that services have the expected number of endpoints
func CheckServicesEndpoints(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ES services should have endpoints",
		Test: test.Eventually(func() error {
			expectedEs := b.GetExpectedElasticsearch()
			for endpointName, addrCount := range map[string]int{
				// we intentionally hardcode the names here to catch any accidental breaking change
				b.Elasticsearch.Name + "-es-http":      int(expectedEs.Spec.NodeCount()),
				b.Elasticsearch.Name + "-es-transport": int(expectedEs.Spec.NodeCount()),
			} {
				if addrCount == 0 {
					continue // maybe no Kibana
				}
				endpoints, err := k.GetEndpoints(b.Elasticsearch.Namespace, endpointName)
				if err != nil {
					return err
				}
				if len(endpoints.Subsets) == 0 {
					return fmt.Errorf("no subset for endpoint %s", endpointName)
				}
				if len(endpoints.Subsets[0].Addresses) != addrCount {
					return fmt.Errorf("%d addresses found for endpoint %s, expected %d", len(endpoints.Subsets[0].Addresses), endpointName, addrCount)
				}
			}
			return nil
		}),
	}
}

// CheckESPassword checks that the user password to access ES is correctly set
func CheckESPassword(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Elastic password should be available",
		Test: test.Eventually(func() error {
			password, err := k.GetElasticPassword(k8s.ExtractNamespacedName(&b.Elasticsearch))
			if err != nil {
				return err
			}
			if password == "" {
				return fmt.Errorf("user password is not set")
			}
			return nil
		}),
	}
}

// CheckClusterUUIDAnnotation waits until the Elasticsearch resource eventually gets annotated
// with the Cluster UUID.
// When the test suite is performing a mutation, it allows making sure the cluster is bootstrapped
// before moving on with the mutation.
func CheckClusterUUIDAnnotation(es esv1.Elasticsearch, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Cluster should be annotated with its UUID once bootstrapped",
		Test: test.Eventually(func() error {
			var retrievedES esv1.Elasticsearch
			if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&es), &retrievedES); err != nil {
				return err
			}
			if !bootstrap.AnnotatedForBootstrap(retrievedES) {
				return errors.New("no bootstrap annotation set")
			}
			return nil
		}),
	}
}

func CheckExpectedPodsEventuallyReady(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "All expected Pods should eventually be ready",
		Test: test.UntilSuccess(func() error {
			return checkExpectedPodsReady(b, k)
		}, RollingUpgradeTimeout),
	}
}

// checkExpectedPodsReady checks that all expected Pods (no more, no less) are there, ready,
// and that any rolling upgrade is over.
// It does not check the entire spec of the Pods.
func checkExpectedPodsReady(b Builder, k *test.K8sClient) error {
	es := b.GetExpectedElasticsearch()
	// check StatefulSets are expected
	if err := checkStatefulSetsReplicas(es, k); err != nil {
		return err
	}
	expectedNodeSets := es.Spec.NodeSets
	// for each StatefulSet, make sure all Pods are there and Ready
	for _, nodeSet := range expectedNodeSets {
		// retrieve the corresponding StatefulSet
		var statefulSet appsv1.StatefulSet
		if err := k.Client.Get(context.Background(),
			types.NamespacedName{
				Namespace: es.Namespace,
				Name:      esv1.StatefulSet(es.Name, nodeSet.Name),
			},
			&statefulSet,
		); err != nil {
			return err
		}
		// the exact expected list of Pods (no more, no less) should exist
		expectedPodNames := sset.PodNames(statefulSet)
		actualPods, err := sset.GetActualPodsForStatefulSet(k.Client, k8s.ExtractNamespacedName(&statefulSet))
		if err != nil {
			return err
		}
		actualPodNames := make([]string, 0, len(actualPods))
		for _, p := range actualPods {
			actualPodNames = append(actualPodNames, p.Name)
		}
		// sort alphabetically for comparison purposes
		sort.Strings(expectedPodNames)
		sort.Strings(actualPodNames)
		if !reflect.DeepEqual(expectedPodNames, actualPodNames) {
			return fmt.Errorf("invalid Pods for StatefulSet %s: expected %v, got %v", statefulSet.Name, expectedPodNames, actualPodNames)
		}

		expectedHash := nodeSetHash(es, nodeSet)
		// all Pods should be running and ready
		for _, p := range actualPods {
			if !k8s.IsPodReady(p) {
				// pretty-print status JSON
				statusJSON, err := json.MarshalIndent(p.Status, "", "    ")
				if err != nil {
					return err
				}
				return fmt.Errorf("pod %s is not Ready.\nStatus:%s", p.Name, statusJSON)
			}

			if err := test.ValidateBuilderHashAnnotation(p, expectedHash); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkStatefulSetsReplicas(es esv1.Elasticsearch, k *test.K8sClient) error {
	// build names and replicas count of expected StatefulSets
	expected := make(map[string]int32, len(es.Spec.NodeSets)) // map[StatefulSetName]Replicas
	for _, nodeSet := range es.Spec.NodeSets {
		expected[esv1.StatefulSet(es.Name, nodeSet.Name)] = nodeSet.Count
	}
	statefulSets, err := k.GetESStatefulSets(es.Namespace, es.Name)
	if err != nil {
		return err
	}
	// compare with actual StatefulSets
	actual := make(map[string]int32, len(statefulSets)) // map[StatefulSetName]Replicas
	for _, statefulSet := range statefulSets {
		actual[statefulSet.Name] = *statefulSet.Spec.Replicas // should not be nil
	}
	if !reflect.DeepEqual(expected, actual) {
		return fmt.Errorf("invalid StatefulSets: expected %v, got %v", expected, actual)
	}
	return nil
}

func AnnotatePodsWithBuilderHash(b Builder, k *test.K8sClient) []test.Step {
	return []test.Step{
		{
			Name: "Annotate Pods with a hash of their Builder spec",
			Test: test.Eventually(func() error {
				es := b.Elasticsearch
				for _, nodeSet := range b.Elasticsearch.Spec.NodeSets {
					pods, err := sset.GetActualPodsForStatefulSet(k.Client, types.NamespacedName{
						Namespace: es.Namespace,
						Name:      esv1.StatefulSet(es.Name, nodeSet.Name),
					})
					if err != nil {
						return err
					}
					for _, pod := range pods {
						if err := test.AnnotatePodWithBuilderHash(k, pod, nodeSetHash(es, nodeSet)); err != nil {
							return err
						}
					}
				}
				return nil
			}),
		},
	}
}

// nodeSetHash builds a hash of the nodeSet specification in the given ES resource.
func nodeSetHash(es esv1.Elasticsearch, nodeSet esv1.NodeSet) string {
	// Normalize the count to zero to exclude it from the hash. Otherwise scaling up/down would affect the hash but
	// existing nodes not affected by the scaling will not be cycled and therefore be annotated with the previous hash.
	nodeSet.Count = 0
	specHash := hash.HashObject(nodeSet)
	esVersionHash := hash.HashObject(es.Spec.Version)
	// normalize the certificate secret name. It has no effect on the Pods as Elasticsearch loads certs dynamically.
	es.Spec.HTTP.TLS.Certificate.SecretName = ""
	httpServiceHash := hash.HashObject(es.Spec.HTTP)
	return hash.HashObject(specHash + esVersionHash + httpServiceHash)
}
