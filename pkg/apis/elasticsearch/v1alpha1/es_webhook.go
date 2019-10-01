package v1alpha1

import (
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// https://book.kubebuilder.io/cronjob-tutorial/webhook-implementation.html

// +kubebuilder:webhook:path=/validate-v1-alpha1-elasticsearch,mutating=false,failurePolicy=ignore,groups=elasticsearch.k8s.elastic.co,resources=elasticsearches,verbs=create;update,versions=v1alpha1,name=elastic-es-validation
func (r *Elasticsearch) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

var eslog = logf.Log.WithName("es-resource")
var _ webhook.Validator = &Elasticsearch{}

func (r *Elasticsearch) ValidateCreate() error {
	eslog.Info("validate create", "name", r.Name)
	return r.validateElasticsearch()
}

func (r *Elasticsearch) ValidateDelete() error {
	// ValidateDelete implements webhook.Validator, but we do not actually validate deletes
	return nil
}

func (r *Elasticsearch) ValidateUpdate(old runtime.Object) error {
	eslog.Info("validate update", "name", r.Name)
	oldEs, ok := old.(*Elasticsearch)
	if !ok {
		return errors.New("Cannot cast old object to Elasticsearch type")
	}
	var errs field.ErrorList

	for _, val := range updateValidations {
		if err := val(oldEs, r); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "elasticsearch.k8s.elastic.co", Kind: "Elasticsearch"},
			r.Name, errs)
	}
	return r.validateElasticsearch()
}

// // Validation is a function from a currently stored Elasticsearch spec and proposed new spec
// // (both inside a Context struct) to a validation.Result.
// type Validation func(ctx Context) validation.Result

// // ElasticsearchVersion groups an ES resource and its parsed version.
// type ElasticsearchVersion struct {
// 	Elasticsearch estype.Elasticsearch
// 	Version       version.Version
// }

// TODO SABO copy things from pkg/controller/elasticsearch/validation
// update them to take receivers of ES type
// and return field.Invalid error types
// add call to webhook start in cmd/main.go
// update kustomize to generate fields

func (r *Elasticsearch) validateElasticsearch() error {
	var errs field.ErrorList
	for _, val := range validations {
		if err := val(r); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "elasticsearch.k8s.elastic.co", Kind: "Elasticsearch"},
		r.Name, errs)
}

func (r *Elasticsearch) validateEsName() *field.Error {
	return nil
}

// func (r *CronJob) validateCronJobName() *field.Error {
//     if len(r.ObjectMeta.Name) > validationutils.DNS1035LabelMaxLength-11 {
//         // The job name length is 63 character like all Kubernetes objects
//         // (which must fit in a DNS subdomain). The cronjob controller appends
//         // a 11-character suffix to the cronjob (`-$TIMESTAMP`) when creating
//         // a job. The job name length limit is 63 characters. Therefore cronjob
//         // names must have length <= 63-11=52. If we don't validate this here,
//         // then job creation will fail later.
//         return field.Invalid(field.NewPath("metadata").Child("name"), r.Name, "must be no more than 52 characters")
//     }
//     return nil
// }
