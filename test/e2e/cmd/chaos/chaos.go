package chaos

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"time"

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
		err := fmt.Errorf("operator namespace must be specified using %s", "operator-namespace")
		log.Error(err, "Required configuration missing")
		return err
	}

	if flags.operatorName == "" {
		err := fmt.Errorf("operator name must be specified using %s", "operator-name")
		log.Error(err, "Required configuration missing")
		return err
	}

	client, err := createClient()
	if err != nil {
		return err
	}

	checkLeaderTicker := time.NewTicker(checkLeaderDelay)
	deletePodTicker := time.NewTicker(flags.deleteOperatorPodDelay)
	changeOperatorReplicasTicker := time.NewTicker(flags.changeOperatorReplicasDelay)

	log.Info("Starting Chaos process", "settings", viper.AllSettings())
	signalChan := signals.SetupSignalHandler()
	for {
		select {
		case <-signalChan:
			log.Info("Signal received: shutting down")
			return nil

		case <-checkLeaderTicker.C:
			operators, err := listOperators(client, flags.operatorNamespace, flags.operatorName)
			if err != nil {
				return err
			}
			if err := checkElectedOperator(operators.Items); err != nil {
				return nil
			}

		case <-deletePodTicker.C:
			operators, err := listOperators(client, flags.operatorNamespace, flags.operatorName)
			if err != nil {
				return err
			}
			if len(operators.Items) == 0 {
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

// checkElectedOperator attempts to ensure that there is at most one operator which is the elected leader.
// It is only done on a best effort basis as it is not possible to have a consistent view of the system at a given
// point in time. We have to be lenient on errors since Pods are restarted frequently.
func checkElectedOperator(pods []corev1.Pod) error {
	var elected []string
	for _, pod := range pods {
		if isElected(pod) {
			elected = append(elected, pod.Name)
		}
	}
	log.Info("Elected operator", "elected", elected, "all", podsName(pods))
	if len(elected) > 1 {
		err := errors.New("several operator instances are elected")
		log.Error(err, "Error while checking which operator is running as elected", "elected", elected)
		return err
	}
	return nil
}

// isElected checks if a Pod is elected.
// It is best effort, we might have some false positive here if the check is done in a middle of a new election.
// We are lenient on errors to not raise a false positive or stop the process too often.
func isElected(pod corev1.Pod) bool {
	if len(pod.Status.PodIP) == 0 {
		return false
	}
	url := fmt.Sprintf("http://%s:9090/metrics", pod.Status.PodIP)
	client := http.Client{
		Timeout: 1 * time.Second,
	}
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

func createClient() (*kubernetes.Clientset, error) {
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
