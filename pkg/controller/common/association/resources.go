package association

import (
	"context"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreatedBy returns true if an object has been created by an other one
type CreatedBy func(created, creator metav1.Object) bool

// DeleteOrphanedResources deletes resources created by an association that are left over from previous reconciliation
// attempts. Common use case is an Elasticsearch reference in Kibana or APMServer spec that was removed.
func DeleteOrphanedResources(
	ctx context.Context,
	c k8s.Client,
	associated commonv1.Associated,
	matchLabels client.MatchingLabels,
	hasBeenCreatedBy CreatedBy,
) error {
	span, _ := apm.StartSpan(ctx, "delete_orphaned_resources", tracing.SpanTypeApp)
	defer span.End()

	var secrets corev1.SecretList
	if err := c.List(&secrets, matchLabels); err != nil {
		return err
	}

	esRef := associated.ElasticsearchRef().WithDefaultNamespace(associated.GetNamespace())
	for _, s := range secrets.Items {
		if err := deleteIfOrphaned(c, &s, associated, hasBeenCreatedBy, esRef); err != nil {
			return err
		}
	}
	return nil
}

func deleteIfOrphaned(
	c k8s.Client,
	secret *corev1.Secret,
	associated commonv1.Associated,
	createdBy CreatedBy,
	esRef commonv1.ObjectSelector,
) error {
	if metav1.IsControlledBy(secret, associated) || createdBy(secret, associated) {
		if !esRef.IsDefined() {
			// look for association secrets owned by this associated instance
			// which should not exist since no ES referenced in the spec
			return deleteExistingSecret(c, secret, associated)
		} else if value, ok := secret.Labels[common.TypeLabelName]; ok && value == user.UserType && esRef.Namespace != secret.Namespace {
			// User secret may live in an other namespace, check if it has changed
			return deleteExistingSecret(c, secret, associated)
		}
	}
	return nil
}

func deleteExistingSecret(c k8s.Client, secret *corev1.Secret, associated commonv1.Associated) error {
	log.Info("Deleting secret", "namespace", secret.Namespace, "secret_name", secret.Name, "associated_name", associated.GetName())
	if err := c.Delete(secret); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}
