// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package helpers

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"os"

	apmtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	assoctype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/associations/v1alpha1"
	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/apmserver"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/nodecerts"
	kblabel "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // auth on gke
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type K8sHelper struct {
	Client k8s.Client
}

func NewK8sClient() (*K8sHelper, error) {
	client, err := CreateClient()
	if err != nil {
		return nil, err
	}
	return &K8sHelper{
		Client: client,
	}, nil
}

func NewK8sClientOrFatal() *K8sHelper {
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
	if err := estype.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	if err := kbtype.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	if err := assoctype.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	if err := apmtype.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	client, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, err
	}
	return k8s.WrapClient(client), nil
}

func (k *K8sHelper) GetPods(listOpts client.ListOptions) ([]corev1.Pod, error) {
	var podList corev1.PodList
	if err := k.Client.List(&listOpts, &podList); err != nil {
		return nil, err
	}
	return podList.Items, nil
}

func (k *K8sHelper) GetPod(name string) (corev1.Pod, error) {
	var pod corev1.Pod
	if err := k.Client.Get(types.NamespacedName{Namespace: params.Namespace, Name: name}, &pod); err != nil {
		return corev1.Pod{}, err
	}
	return pod, nil
}

func (k *K8sHelper) DeletePod(pod corev1.Pod) error {
	return k.Client.Delete(&pod)
}

func (k *K8sHelper) CheckPodCount(listOpts client.ListOptions, expectedCount int) error {
	pods, err := k.GetPods(listOpts)
	if err != nil {
		return err
	}
	actualCount := len(pods)
	if expectedCount != actualCount {
		return fmt.Errorf("Invalid node count: expected %d, got %d", expectedCount, actualCount)
	}
	return nil
}

func (k *K8sHelper) GetService(name string) (*corev1.Service, error) {
	var service corev1.Service
	key := types.NamespacedName{
		Namespace: params.Namespace,
		Name:      name,
	}
	if err := k.Client.Get(key, &service); err != nil {
		return nil, err
	}
	return &service, nil
}

func (k *K8sHelper) GetEndpoints(name string) (*corev1.Endpoints, error) {
	var endpoints corev1.Endpoints
	key := types.NamespacedName{
		Namespace: params.Namespace,
		Name:      name,
	}
	if err := k.Client.Get(key, &endpoints); err != nil {
		return nil, err
	}
	return &endpoints, nil
}

func (k *K8sHelper) GetElasticPassword(stackName string) (string, error) {
	secretName := stackName + "-elastic-user"
	elasticUserKey := "elastic"
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: params.Namespace,
		Name:      secretName,
	}
	if err := k.Client.Get(key, &secret); err != nil {
		return "", err
	}
	password, exists := secret.Data[elasticUserKey]
	if !exists {
		return "", fmt.Errorf("No %s value found for secret %s", elasticUserKey, secretName)
	}
	return string(password), nil
}

func (k *K8sHelper) GetCACert(stackName string) ([]*x509.Certificate, error) {
	secretName := nodecerts.CACertSecretName(stackName)
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: params.Namespace,
		Name:      secretName,
	}
	if err := k.Client.Get(key, &secret); err != nil {
		return nil, err
	}
	caCert, exists := secret.Data[certificates.CAFileName]
	if !exists {
		return nil, fmt.Errorf("no value found for secret %s", certificates.CAFileName)
	}
	return certificates.ParsePEMCerts(caCert)
}

// GetCAPrivateKey returns the private key of the given stack
func (k *K8sHelper) GetCAPrivateKey(stackName string) (*rsa.PrivateKey, error) {
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: params.Namespace,
		Name:      name.CAPrivateKeySecret(stackName),
	}
	if err := k.Client.Get(key, &secret); err != nil {
		return nil, err
	}
	pKeyBytes, exists := secret.Data[nodecerts.CAPrivateKeyFileName]
	if !exists || len(pKeyBytes) == 0 {
		return nil, fmt.Errorf("no value found for secret %s", nodecerts.CAPrivateKeyFileName)
	}
	return certificates.ParsePEMPrivateKey(pKeyBytes)
}

// GetNodeCert retrieves the certificate of the CA and the node certificate
func (k *K8sHelper) GetNodeCert(podName string) (caCert, nodeCert []*x509.Certificate, err error) {
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: params.Namespace,
		Name:      name.CertsSecret(podName),
	}
	if err = k.Client.Get(key, &secret); err != nil {
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
	nodeCertBytes, exists := secret.Data[nodecerts.CertFileName]
	if !exists || len(nodeCertBytes) == 0 {
		return nil, nil, fmt.Errorf("no value found for secret %s", nodecerts.CertFileName)
	}
	nodeCert, err = certificates.ParsePEMCerts(nodeCertBytes)
	if err != nil {
		return nil, nil, err
	}
	return
}

// Exec runs the given cmd into the given pod.
func (k *K8sHelper) Exec(pod types.NamespacedName, cmd []string) (string, string, error) {
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

func ESPodListOptions(stackName string) client.ListOptions {
	return client.ListOptions{
		Namespace: params.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{
			common.TypeLabelName:       label.Type,
			label.ClusterNameLabelName: stackName,
		}))}
}

func KibanaPodListOptions(stackName string) client.ListOptions {
	return client.ListOptions{
		Namespace: params.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{
			kblabel.KibanaNameLabelName: stackName,
		}))}
}

func ApmServerPodListOptions(stackName string) client.ListOptions {
	return client.ListOptions{
		Namespace: params.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{
			common.TypeLabelName:             apmserver.Type,
			apmserver.ApmServerNameLabelName: stackName,
		}))}
}

func GetFirstPodMatching(pods []corev1.Pod, predicate func(pod corev1.Pod) bool) (corev1.Pod, bool) {
	for _, pod := range pods {
		if predicate(pod) {
			return pod, true
		}
	}
	return corev1.Pod{}, false
}
