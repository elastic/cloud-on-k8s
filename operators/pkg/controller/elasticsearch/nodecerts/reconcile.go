package nodecerts

import (
	"errors"
	"fmt"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.KBLog.WithName("nodecerts")

func ReconcileNodeCertificateSecrets(
	c k8s.Client,
	ca *nodecerts.Ca,
	es v1alpha1.ElasticsearchCluster,
	services []corev1.Service,
) (reconcile.Result, error) {
	log.Info("Reconciling node certificate secrets")

	nodeCertificateSecrets, err := findNodeCertificateSecrets(c, es)
	if err != nil {
		return reconcile.Result{}, err
	}

	for _, secret := range nodeCertificateSecrets {
		// todo: error checking if label does not exist
		podName := secret.Labels[nodecerts.LabelAssociatedPod]

		var pod corev1.Pod
		if err := c.Get(types.NamespacedName{Namespace: secret.Namespace, Name: podName}, &pod); err != nil {
			if !apierrors.IsNotFound(err) {
				return reconcile.Result{}, err
			}

			// give some leniency in pods showing up only after a while.
			if secret.CreationTimestamp.Add(5 * time.Minute).Before(time.Now()) {
				// if the secret has existed for too long without an associated pod, it's time to GC it
				log.Info("Unable to find pod associated with secret, GCing", "secret", secret.Name)
				if err := c.Delete(&secret); err != nil {
					return reconcile.Result{}, err
				}
			} else {
				log.Info("Unable to find pod associated with secret, but secret is too young for GC", "secret", secret.Name)
			}
			continue
		}

		if pod.Status.PodIP == "" {
			log.Info("Skipping secret because associated pod has no pod ip", "secret", secret.Name)
			continue
		}

		certificateType, ok := secret.Labels[nodecerts.LabelNodeCertificateType]
		if !ok {
			log.Error(errors.New("missing certificate type"), "No certificate type found", "secret", secret.Name)
			continue
		}

		switch certificateType {
		case nodecerts.LabelNodeCertificateTypeElasticsearchAll:
			if res, err := nodecerts.ReconcileNodeCertificateSecret(
				secret, pod, es.Name, es.Namespace, services, ca, c,
			); err != nil {
				return res, err
			}
		default:
			log.Error(
				errors.New("unsupported certificate type"),
				fmt.Sprintf("Unsupported cerificate type: %s found in %s, ignoring", certificateType, secret.Name),
			)
		}
	}

	return reconcile.Result{}, nil
}

func findNodeCertificateSecrets(
	c k8s.Client,
	es v1alpha1.ElasticsearchCluster,
) ([]corev1.Secret, error) {
	var nodeCertificateSecrets corev1.SecretList
	listOptions := client.ListOptions{
		Namespace: es.Namespace,
		LabelSelector: labels.Set(map[string]string{
			label.ClusterNameLabelName: es.Name,
			nodecerts.LabelSecretUsage: nodecerts.LabelSecretUsageNodeCertificates,
		}).AsSelector(),
	}

	if err := c.List(&listOptions, &nodeCertificateSecrets); err != nil {
		return nil, err
	}

	return nodeCertificateSecrets.Items, nil
}
