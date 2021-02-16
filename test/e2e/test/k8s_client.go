// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"bytes"
	"context"
	"crypto/x509"
	"fmt"
	"os"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	entv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/agent"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver"
	beatcommon "github.com/elastic/cloud-on-k8s/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/enterprisesearch"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // auth on gke
	"k8s.io/client-go/tools/remotecommand"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type K8sClient struct {
	Client k8s.Client
}

func NewK8sClient() (*K8sClient, error) {
	client, err := CreateClient()
	if err != nil {
		return nil, err
	}
	return &K8sClient{
		Client: client,
	}, nil
}

func NewK8sClientOrFatal() *K8sClient {
	client, err := NewK8sClient()
	if err != nil {
		fmt.Println("Cannot create K8s client", err)
		os.Exit(1)
	}
	return client
}

func CreateClient() (k8s.Client, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	if err := esv1.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	if err := kbv1.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	if err := apmv1.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	if err := beatv1beta1.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	if err := entv1beta1.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	if err := agentv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	client, err := k8sclient.New(cfg, k8sclient.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, err
	}
	return client, nil
}

func ServerVersion() (*version.Info, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "while getting rest config")
	}
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "while creating discovery client")
	}
	return dc.ServerVersion()
}

func (k *K8sClient) GetPods(opts ...k8sclient.ListOption) ([]corev1.Pod, error) {
	var podList corev1.PodList
	if err := k.Client.List(context.Background(), &podList, opts...); err != nil {
		return nil, err
	}
	return podList.Items, nil
}

func (k *K8sClient) GetPod(namespace, name string) (corev1.Pod, error) {
	var pod corev1.Pod
	if err := k.Client.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, &pod); err != nil {
		return corev1.Pod{}, err
	}
	return pod, nil
}

func (k *K8sClient) GetESStatefulSets(namespace string, esName string) ([]appsv1.StatefulSet, error) {
	var ssetList appsv1.StatefulSetList
	if err := k.Client.List(context.Background(), &ssetList,
		k8sclient.InNamespace(namespace),
		k8sclient.MatchingLabels{
			label.ClusterNameLabelName: esName,
		}); err != nil {
		return nil, err
	}
	return ssetList.Items, nil
}

func (k *K8sClient) DeletePod(pod corev1.Pod) error {
	return k.Client.Delete(context.Background(), &pod)
}

func (k *K8sClient) CheckPodCount(expectedCount int, opts ...k8sclient.ListOption) error {
	pods, err := k.GetPods(opts...)
	if err != nil {
		return err
	}
	actualCount := len(pods)
	if expectedCount != actualCount {
		return fmt.Errorf("invalid node count: expected %d, got %d", expectedCount, actualCount)
	}
	return nil
}

func (k *K8sClient) GetService(namespace, name string) (*corev1.Service, error) {
	var service corev1.Service
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	if err := k.Client.Get(context.Background(), key, &service); err != nil {
		return nil, err
	}
	return &service, nil
}

func (k *K8sClient) GetEndpoints(namespace, name string) (*corev1.Endpoints, error) {
	var endpoints corev1.Endpoints
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	if err := k.Client.Get(context.Background(), key, &endpoints); err != nil {
		return nil, err
	}
	return &endpoints, nil
}

func (k *K8sClient) GetEvents(opts ...k8sclient.ListOption) ([]corev1.Event, error) {
	var eventList corev1.EventList
	if err := k.Client.List(context.Background(), &eventList, opts...); err != nil {
		return nil, err
	}
	return eventList.Items, nil
}

func (k *K8sClient) GetElasticPassword(nsn types.NamespacedName) (string, error) {
	secretName := nsn.Name + "-es-elastic-user"
	elasticUserKey := "elastic"
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: nsn.Namespace,
		Name:      secretName,
	}
	if err := k.Client.Get(context.Background(), key, &secret); err != nil {
		return "", err
	}
	password, exists := secret.Data[elasticUserKey]
	if !exists {
		return "", fmt.Errorf("no %s value found for secret %s", elasticUserKey, secretName)
	}
	return string(password), nil
}

func (k *K8sClient) GetHTTPCerts(namer name.Namer, ownerNamespace, ownerName string) ([]*x509.Certificate, error) {
	var secret corev1.Secret
	secretNSN := certificates.PublicCertsSecretRef(
		namer,
		types.NamespacedName{
			Namespace: ownerNamespace,
			Name:      ownerName,
		},
	)

	if err := k.Client.Get(context.Background(),
		secretNSN,
		&secret,
	); err != nil {
		return nil, err
	}

	certData, exists := secret.Data[certificates.CertFileName]
	if !exists {
		return nil, fmt.Errorf("no certificates found in secret %s", secretNSN)
	}
	return certificates.ParsePEMCerts(certData)
}

// GetCA returns the CA of the given owner name
func (k *K8sClient) GetCA(ownerNamespace, ownerName string, caType certificates.CAType) (*certificates.CA, error) {
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: ownerNamespace,
		Name:      certificates.CAInternalSecretName(esv1.ESNamer, ownerName, caType),
	}
	if err := k.Client.Get(context.Background(), key, &secret); err != nil {
		return nil, err
	}

	caCertsData, exists := secret.Data[certificates.CertFileName]
	if !exists {
		return nil, fmt.Errorf("no value found for cert in secret %s", certificates.CertFileName)
	}
	caCerts, err := certificates.ParsePEMCerts(caCertsData)
	if err != nil {
		return nil, err
	}

	if len(caCerts) != 1 {
		return nil, fmt.Errorf("found multiple ca certificates in secret %s", key)
	}

	pKeyBytes, exists := secret.Data[certificates.KeyFileName]
	if !exists || len(pKeyBytes) == 0 {
		return nil, fmt.Errorf("no value found for private key in secret %s", key)
	}
	pKey, err := certificates.ParsePEMPrivateKey(pKeyBytes)
	if err != nil {
		return nil, err
	}

	return certificates.NewCA(pKey, caCerts[0]), nil
}

// Exec runs the given cmd into the given pod.
func (k *K8sClient) Exec(pod types.NamespacedName, cmd []string) (string, string, error) {
	// create the exec client
	cfg, err := config.GetConfig()
	if err != nil {
		return "", "", err
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "", "", err
	}

	// build the exec url
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec").
		VersionedParams(
			&corev1.PodExecOptions{
				Command: cmd,
				Stdout:  true,
				Stderr:  true,
			},
			runtime.NewParameterCodec(scheme.Scheme),
		)
	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		return "", "", err
	}

	// execute
	var stdout, stderr bytes.Buffer
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})
	return stdout.String(), stderr.String(), err
}

func (k *K8sClient) CheckSecretsRemoved(secretRefs []types.NamespacedName) error {
	for _, ref := range secretRefs {
		err := k.Client.Get(context.Background(), ref, &corev1.Secret{})
		if apierrors.IsNotFound(err) {
			// secret removed, all good
			continue
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("expected secret %s to be garbage-collected", ref.Name)
	}
	return nil
}

func ESPodListOptions(esNamespace, esName string) []k8sclient.ListOption {
	ns := k8sclient.InNamespace(esNamespace)
	matchLabels := k8sclient.MatchingLabels(map[string]string{
		common.TypeLabelName:       label.Type,
		label.ClusterNameLabelName: esName,
	})
	return []k8sclient.ListOption{ns, matchLabels}
}

func KibanaPodListOptions(kbNamespace, kbName string) []k8sclient.ListOption {
	ns := k8sclient.InNamespace(kbNamespace)
	matchLabels := k8sclient.MatchingLabels(map[string]string{
		kibana.KibanaNameLabelName: kbName,
	})
	return []k8sclient.ListOption{ns, matchLabels}
}

func ApmServerPodListOptions(apmNamespace, apmName string) []k8sclient.ListOption {
	ns := k8sclient.InNamespace(apmNamespace)
	matchLabels := k8sclient.MatchingLabels(map[string]string{
		common.TypeLabelName:             apmserver.Type,
		apmserver.ApmServerNameLabelName: apmName,
	})
	return []k8sclient.ListOption{ns, matchLabels}
}

func EnterpriseSearchPodListOptions(entNamespace, entName string) []k8sclient.ListOption {
	ns := k8sclient.InNamespace(entNamespace)
	matchLabels := k8sclient.MatchingLabels(map[string]string{
		common.TypeLabelName:                           enterprisesearch.Type,
		enterprisesearch.EnterpriseSearchNameLabelName: entName,
	})
	return []k8sclient.ListOption{ns, matchLabels}
}

func AgentPodListOptions(agentNamespace, agentName string) []k8sclient.ListOption {
	ns := k8sclient.InNamespace(agentNamespace)
	matchLabels := k8sclient.MatchingLabels(map[string]string{
		common.TypeLabelName: agent.TypeLabelValue,
		agent.NameLabelName:  agent.Name(agentName),
	})
	return []k8sclient.ListOption{ns, matchLabels}
}

func BeatPodListOptions(beatNamespace, beatName, beatType string) []k8sclient.ListOption {
	ns := k8sclient.InNamespace(beatNamespace)
	matchLabels := k8sclient.MatchingLabels(map[string]string{
		common.TypeLabelName:     beatcommon.TypeLabelValue,
		beatcommon.NameLabelName: beatcommon.Name(beatName, beatType),
	})
	return []k8sclient.ListOption{ns, matchLabels}
}

func EventListOptions(namespace, name string) []k8sclient.ListOption {
	ns := k8sclient.InNamespace(namespace)
	matchFields := k8sclient.MatchingFields(map[string]string{
		"involvedObject.name":      name,
		"involvedObject.namespace": namespace,
	})
	return []k8sclient.ListOption{ns, matchFields}
}

func GetFirstPodMatching(pods []corev1.Pod, predicate func(pod corev1.Pod) bool) (corev1.Pod, bool) {
	for _, pod := range pods {
		if predicate(pod) {
			return pod, true
		}
	}
	return corev1.Pod{}, false
}

func OnAllPods(pods []corev1.Pod, f func(corev1.Pod) error) error {
	// map phase: execute a function on all pods in parallel
	fResults := make(chan error, len(pods))
	for _, p := range pods {
		go func(pod corev1.Pod) {
			fResults <- f(pod)
		}(p)
	}
	// reduce phase: aggregate errors (simply return the last one seen)
	var err error
	for range pods {
		podErr := <-fResults
		if podErr != nil {
			err = podErr
		}
	}
	return err
}
