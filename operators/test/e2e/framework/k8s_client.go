// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package framework

import (
	"bytes"
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
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/certificates/http"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	kblabel "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
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

type K8sClient struct {
	Client k8s.Client
}

func NewK8sClient() (*K8sClient, error) {
	kClient, err := CreateClient()
	if err != nil {
		return nil, err
	}
	return &K8sClient{
		Client: kClient,
	}, nil
}

func NewK8sClientOrFatal() *K8sClient {
	kClient, err := NewK8sClient()
	if err != nil {
		fmt.Println("Cannot create K8s client", err)
		os.Exit(1)
	}
	return kClient
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
	kClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, err
	}
	return k8s.WrapClient(kClient), nil
}

func (k *K8sClient) GetPods(listOpts client.ListOptions) ([]corev1.Pod, error) {
	var podList corev1.PodList
	if err := k.Client.List(&listOpts, &podList); err != nil {
		return nil, err
	}
	return podList.Items, nil
}

func (k *K8sClient) GetPod(name string) (corev1.Pod, error) {
	var pod corev1.Pod
	if err := k.Client.Get(types.NamespacedName{Namespace: Namespace, Name: name}, &pod); err != nil {
		return corev1.Pod{}, err
	}
	return pod, nil
}

func (k *K8sClient) DeletePod(pod corev1.Pod) error {
	return k.Client.Delete(&pod)
}

func (k *K8sClient) CheckPodCount(listOpts client.ListOptions, expectedCount int) error {
	pods, err := k.GetPods(listOpts)
	if err != nil {
		return err
	}
	actualCount := len(pods)
	if expectedCount != actualCount {
		return fmt.Errorf("invalid node count: expected %d, got %d", expectedCount, actualCount)
	}
	return nil
}

func (k *K8sClient) GetService(name string) (*corev1.Service, error) {
	var service corev1.Service
	key := types.NamespacedName{
		Namespace: Namespace,
		Name:      name,
	}
	if err := k.Client.Get(key, &service); err != nil {
		return nil, err
	}
	return &service, nil
}

func (k *K8sClient) GetEndpoints(name string) (*corev1.Endpoints, error) {
	var endpoints corev1.Endpoints
	key := types.NamespacedName{
		Namespace: Namespace,
		Name:      name,
	}
	if err := k.Client.Get(key, &endpoints); err != nil {
		return nil, err
	}
	return &endpoints, nil
}

func (k *K8sClient) GetElasticPassword(esName string) (string, error) {
	secretName := esName + "-es-elastic-user"
	elasticUserKey := "elastic"
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: Namespace,
		Name:      secretName,
	}
	if err := k.Client.Get(key, &secret); err != nil {
		return "", err
	}
	password, exists := secret.Data[elasticUserKey]
	if !exists {
		return "", fmt.Errorf("no %s value found for secret %s", elasticUserKey, secretName)
	}
	return string(password), nil
}

func (k *K8sClient) GetHTTPCerts(esName string) ([]*x509.Certificate, error) {
	var secret corev1.Secret
	secretNSN := http.PublicCertsSecretRef(
		types.NamespacedName{
			Namespace: Namespace,
			Name:      esName,
		},
	)

	if err := k.Client.Get(
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
func (k *K8sClient) GetCA(ownerName string, caType certificates.CAType) (*certificates.CA, error) {
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: Namespace,
		Name:      certificates.CAInternalSecretName(name.ESNamer, ownerName, caType),
	}
	if err := k.Client.Get(key, &secret); err != nil {
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

// GetTransportCert retrieves the certificate of the CA and the transport certificate
func (k *K8sClient) GetTransportCert(podName string) (caCert, transportCert []*x509.Certificate, err error) {
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: Namespace,
		Name:      name.TransportCertsSecret(podName),
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
	transportCertBytes, exists := secret.Data[certificates.CertFileName]
	if !exists || len(transportCertBytes) == 0 {
		return nil, nil, fmt.Errorf("no value found for secret %s", certificates.CertFileName)
	}
	transportCert, err = certificates.ParsePEMCerts(transportCertBytes)
	if err != nil {
		return nil, nil, err
	}
	return
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

func ESPodListOptions(esName string) client.ListOptions {
	return client.ListOptions{
		Namespace: Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{
			common.TypeLabelName:       label.Type,
			label.ClusterNameLabelName: esName,
		}))}
}

func KibanaPodListOptions(kbName string) client.ListOptions {
	return client.ListOptions{
		Namespace: Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{
			kblabel.KibanaNameLabelName: kbName,
		}))}
}

func ApmServerPodListOptions(apmName string) client.ListOptions {
	return client.ListOptions{
		Namespace: Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{
			common.TypeLabelName:             apmserver.Type,
			apmserver.ApmServerNameLabelName: apmName,
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
