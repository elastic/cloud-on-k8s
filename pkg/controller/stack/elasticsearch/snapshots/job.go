package snapshots

import (
	"path"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/pkg/controller/stack/common/nodecerts"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"

	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// CronJobParams describe parameters to construct a snapshotter job.
type CronJobParams struct {
	Parent types.NamespacedName
	// TODO refactor to just use namespaced Name
	Stack            deploymentsv1alpha1.Stack
	SnapshotterImage string
	User             client.User
	EsURL            string
}

// CronJobName returns the name of the cronjob for the given parent resource (stack).
func CronJobName(parent types.NamespacedName) string {
	return common.Concat(parent.Name, "-snapshotter")
}

// NewCronJob constructor for snapshotter cronjobs.
func NewCronJob(params CronJobParams) *batchv1beta1.CronJob {
	parallism := int32(1)
	completions := int32(1)
	// TODO brittle, by convention currently called like the stack
	caCertSecret := elasticsearch.NewSecretVolume(params.Parent.Name, "ca")
	certPath := path.Join(elasticsearch.DefaultSecretMountPath, nodecerts.SecretCAKey)

	meta := metav1.ObjectMeta{
		Namespace: params.Parent.Namespace,
		Name:      CronJobName(params.Parent),
		Labels:    elasticsearch.NewLabels(params.Stack),
	}

	return &batchv1beta1.CronJob{
		ObjectMeta: meta,
		Spec: batchv1beta1.CronJobSpec{
			Schedule: "*/10 * * * *",
			JobTemplate: batchv1beta1.JobTemplateSpec{
				ObjectMeta: meta,
				Spec: batchv1.JobSpec{
					Parallelism: &parallism,
					Completions: &completions,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: meta,
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{{
								Env: []corev1.EnvVar{
									{Name: "CERTIFICATE_LOCATION", Value: certPath},
									{Name: "ELASTICSEARCH_URL", Value: params.EsURL},
									{Name: "USER", Value: params.User.Name},
									{Name: "PASSWORD", ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: elasticsearch.ElasticInternalUsersSecretName(params.Parent.Name),
											},
											Key: params.User.Name,
										},
									}},
								},
								Image:           params.SnapshotterImage,
								ImagePullPolicy: corev1.PullIfNotPresent,
								Name:            CronJobName(params.Parent),
								VolumeMounts: []corev1.VolumeMount{
									caCertSecret.VolumeMount(),
								},
							}},
							Volumes: []corev1.Volume{
								caCertSecret.Volume(),
							},
						},
					},
				},
			},
		},
	}

}
