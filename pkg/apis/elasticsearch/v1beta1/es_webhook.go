package v1beta1

import (
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// +kubebuilder:webhook:path=/validate-elasticsearch,mutating=false,failurePolicy=ignore,groups=elasticsearch.k8s.elastic.co,resources=elasticsearches,verbs=create;update,versions=v1beta1,name=elastic-es-validation

func (r *Elasticsearch) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

var eslog = logf.Log.WithName("es-validation")

var _ webhook.Validator = &Elasticsearch{}

func (r *Elasticsearch) ValidateCreate() error {
	eslog.Info("validate create", "name", r.Name)
	return r.validateElasticsearch()
}

// ValidateDelete is required to implement webhook.Validator, but we do not actually validate deletes
func (r *Elasticsearch) ValidateDelete() error {
	return nil
}

func (r *Elasticsearch) ValidateUpdate(old runtime.Object) error {
	eslog.Info("validate update", "name", r.Name)
	oldEs, ok := old.(*Elasticsearch)
	if !ok {
		return errors.New("cannot cast old object to Elasticsearch type")
	}
	var errs field.ErrorList

	for _, val := range updateValidations {
		if err := val(oldEs, r); err != nil {
			errs = append(errs, err...)
		}
	}
	if len(errs) > 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "elasticsearch.k8s.elastic.co", Kind: "Elasticsearch"},
			r.Name, errs)
	}
	return r.validateElasticsearch()
}

func (r *Elasticsearch) validateElasticsearch() error {
	var errs field.ErrorList
	for _, val := range validations {
		if err := val(r); err != nil {
			errs = append(errs, err...)
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "elasticsearch.k8s.elastic.co", Kind: "Elasticsearch"},
		r.Name, errs)
}
