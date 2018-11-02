package kibana

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ServiceName(stackName string) string {
	return stackName + "-kb"
}

func NewService(namespace string, stackName string, stackID string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      ServiceName(stackName),
			Labels:    NewLabelsWithStackID(stackID),
		},
		Spec: corev1.ServiceSpec{
			Selector: NewLabelsWithStackID(stackID),
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     HTTPPort,
				},
			},
			SessionAffinity: corev1.ServiceAffinityNone,
			// For now, expose the service as node port to ease development
			// TODO: proper ingress forwarding
			Type:                  corev1.ServiceTypeNodePort,
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
		},
	}

}
