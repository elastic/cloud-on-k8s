package kibana

import (
	kibanav1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewService(kb kibanav1alpha1.Kibana) *corev1.Service {
	var svc = corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: kb.Namespace,
			Name:      PseudoNamespacedResourceName(kb),
			Labels:    NewLabels(kb.Name),
		},
		Spec: corev1.ServiceSpec{
			Selector: NewLabels(kb.Name),
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     HTTPPort,
				},
			},
			SessionAffinity: corev1.ServiceAffinityNone,
			// For now, expose the service as node port to ease development
			// TODO: proper ingress forwarding
			Type: common.GetServiceType(kb.Spec.Expose),
		},
	}
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		svc.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeCluster
	}
	return &svc
}
