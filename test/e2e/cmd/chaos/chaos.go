// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package chaos

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/dev/portforward"
	logconf "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/prometheus/common/expfmt"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

// doRun runs the main chaos process. It randomly deletes operator Pods, scale up and down operator replicas and attempts
// to ensure that only one Pod is running as the elected one.
func doRun(flags runFlags) error {
	logconf.ChangeVerbosity(flags.logVerbosity)

	if flags.operatorNamespace == "" {
		err := errors.New("operator namespace must be specified using --operator-namespace")
		log.Error(err, "Required configuration missing")
		return err
	}

	if flags.operatorName == "" {
		err := errors.New("operator name must be specified using --operator-name")
		log.Error(err, "Required configuration missing")
		return err
	}

	client, err := createK8SClient()
	if err != nil {
		return err
	}

	checkLeaderTicker := time.NewTicker(checkLeaderDelay)
	deletePodTicker := time.NewTicker(flags.deleteOperatorPodDelay)
	changeOperatorReplicasTicker := time.NewTicker(flags.changeOperatorReplicasDelay)

	log.Info("Starting Chaos process", "settings", viper.AllSettings())
	signalCtx := signals.SetupSignalHandler()
	for {
		select {
		case <-signalCtx.Done():
			log.Info("Signal received: shutting down")
			return nil

		case <-checkLeaderTicker.C:
			operators, err := listOperators(client, flags.operatorNamespace, flags.operatorName)
			if err != nil {
				return err
			}
			if err := checkElectedOperator(operators.Items, flags.autoPortForwarding); err != nil {
				return nil
			}

		case <-deletePodTicker.C:
			operators, err := listOperators(client, flags.operatorNamespace, flags.operatorName)
			if err != nil {
				return err
			}
			if len(operators.Items) == 0 {
				log.Info("No operator Pod available for deletion")
				continue
			}
			toDelete := rand.Intn(len(operators.Items))
			podToDelete := operators.Items[toDelete]
			log.Info("Deleting operator", "pod_name", podToDelete.Name)
			if err := client.CoreV1().Pods(flags.operatorNamespace).Delete(context.Background(), podToDelete.Name, metav1.DeleteOptions{}); err != nil {
				log.Error(err, "Error while deleting operator", err, "pod_name", podToDelete.Name)
			}

		case <-changeOperatorReplicasTicker.C:
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				operatorStatefulSet, err := client.AppsV1().StatefulSets(flags.operatorNamespace).Get(context.Background(), flags.operatorName, metav1.GetOptions{})
				if err != nil {
					log.Error(err, "Error while retrieving operator statefulset", err, "sts_name", flags.operatorName)
					return err
				}
				var currentReplicas, newReplicas int32
				if operatorStatefulSet.Spec.Replicas != nil {
					currentReplicas = *operatorStatefulSet.Spec.Replicas
				}

				if currentReplicas != minReplicas {
					newReplicas = minReplicas
				} else {
					newReplicas = maxReplicas
				}
				log.Info("Change operator replicas", "sts_name", flags.operatorName, "current_replicas", currentReplicas, "new_replicas", newReplicas)
				operatorStatefulSet.Spec.Replicas = &newReplicas
				_, err = client.AppsV1().StatefulSets(flags.operatorNamespace).Update(context.Background(), operatorStatefulSet, metav1.UpdateOptions{})
				return err
			})
			if err != nil {
				return err
			}
		}
	}

}

// listOperators retrieves the list of running operator instances.
func listOperators(client *kubernetes.Clientset, operatorNamespace, operatorName string) (*corev1.PodList, error) {
	return client.CoreV1().Pods(operatorNamespace).List(context.Background(),
		metav1.ListOptions{LabelSelector: "control-plane=" + operatorName},
	)
}

var lastOperatorState operatorState

// checkElectedOperator attempts to ensure that there is at most one operator which is the elected leader.
// It is only done on a best effort basis as it is not possible to have a consistent view of the system at a given
// point in time. We have to be lenient on errors since Pods are restarted frequently.
func checkElectedOperator(pods []corev1.Pod, autoPortForwarding bool) error {
	elected := make([]string, 0, len(pods))
	for _, pod := range pods {
		if isElected(pod, autoPortForwarding) {
			elected = append(elected, pod.Name)
		}
	}

	var currentOperatorState operatorState
	switch numElected := len(elected); {
	case numElected > 1:
		err := errors.New("several operator instances are elected")
		log.Error(err, "Error while checking which operator is running as elected", "elected", elected)
		return err
	case numElected == 1:
		currentOperatorState = newOperatorState(pods, elected[0])
	case numElected == 0:
		currentOperatorState = newOperatorState(pods, "")
	}

	if !currentOperatorState.equal(lastOperatorState) {
		log.Info("Elected operator", "elected", elected, "all", podsName(pods))
	}
	lastOperatorState = currentOperatorState
	return nil
}

// isElected checks if a Pod is elected.
// It is best effort, we might have some false positive here if the check is done in a middle of a new election.
// We are lenient on errors to not raise a false positive or stop the process too often.
func isElected(pod corev1.Pod, autoPortForwarding bool) bool {
	if len(pod.Status.PodIP) == 0 {
		return false
	}
	url := fmt.Sprintf("http://%s:9090/metrics", pod.Status.PodIP)

	client := createHTTPClient(autoPortForwarding)
	resp, err := client.Get(url)
	if err != nil {
		// Timeout or closed connections may happen since some Pods might be in the process of being started or stopped.
		log.Error(err, "Error while retrieving Pod metrics")
		return false
	}
	defer resp.Body.Close()

	var parser expfmt.TextParser
	parsed, err := parser.TextToMetricFamilies(resp.Body)
	if err != nil {
		log.Error(err, "Error while parsing Pod metrics")
		return false
	}
	value, ok := parsed["elastic_leader"]
	if !ok {
		return false
	}

	for _, metric := range value.GetMetric() {
		if metric.Gauge.GetValue() > 0 {
			return true
		}
	}

	return false
}

// createHTTPClient creates an HTTP client to connect to the PODs.
func createHTTPClient(autoPortForwarding bool) http.Client {
	if autoPortForwarding {
		transportConfig := http.Transport{}
		// use the custom dialer if provided
		dialer := portforward.NewForwardingDialer()
		transportConfig.DialContext = dialer.DialContext
		return http.Client{
			Transport: &transportConfig,
		}
	}
	return http.Client{Timeout: 1 * time.Second}
}

func createK8SClient() (*kubernetes.Clientset, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

func podsName(pods []corev1.Pod) []string {
	podNames := make([]string, len(pods))
	for i := range pods {
		podNames[i] = pods[i].Name
	}
	return podNames
}
