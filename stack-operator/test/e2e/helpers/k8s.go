package helpers

import (
	"context"
	"fmt"
	"os"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // auth on gke
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const DefaultNamespace = "e2e"

var DefaultCtx = context.TODO()

type K8sHelper struct {
	Client client.Client
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

func CreateClient() (client.Client, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	v1alpha1.AddToScheme(scheme.Scheme)
	client, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (k *K8sHelper) GetPods(listOpts client.ListOptions) ([]corev1.Pod, error) {
	var podList corev1.PodList
	if err := k.Client.List(DefaultCtx, &listOpts, &podList); err != nil {
		return nil, err
	}
	return podList.Items, nil
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
		Namespace: DefaultNamespace,
		Name:      name,
	}
	if err := k.Client.Get(DefaultCtx, key, &service); err != nil {
		return nil, err
	}
	return &service, nil
}

func (k *K8sHelper) GetEndpoints(name string) (*corev1.Endpoints, error) {
	var endpoints corev1.Endpoints
	key := types.NamespacedName{
		Namespace: DefaultNamespace,
		Name:      name,
	}
	if err := k.Client.Get(DefaultCtx, key, &endpoints); err != nil {
		return nil, err
	}
	return &endpoints, nil
}

func (k *K8sHelper) GetElasticPassword(stackName string) (string, error) {
	secretName := stackName + "-elastic-user"
	elasticUserKey := "elastic"
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: DefaultNamespace,
		Name:      secretName,
	}
	if err := k.Client.Get(DefaultCtx, key, &secret); err != nil {
		return "", err
	}
	password, exists := secret.Data[elasticUserKey]
	if !exists {
		return "", fmt.Errorf("No %s value found for secret %s", elasticUserKey, secretName)
	}
	return string(password), nil
}

func ESPodListOptions(stackName string) client.ListOptions {
	return client.ListOptions{
		Namespace: DefaultNamespace,
		LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{
			"common.k8s.elastic.co/type":                "elasticsearch",
			"elasticsearch.k8s.elastic.co/cluster-name": stackName,
		}))}
}

func KibanaPodListOptions(stackName string) client.ListOptions {
	return client.ListOptions{
		Namespace: DefaultNamespace,
		LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{
			"kibana.k8s.elastic.co/name": stackName,
		}))}
}
