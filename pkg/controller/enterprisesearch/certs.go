package enterprisesearch

import (
	"context"
	"time"

	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/enterprisesearch/name"
)

// TODO: refactor with APM

func ReconcileCertificates(
	ctx context.Context,
	driver driver.Interface,
	ents *entsv1beta1.EnterpriseSearch,
	services []corev1.Service,
	caRotation certificates.RotationParams,
	certRotation certificates.RotationParams,
) *reconciler.Results {
	span, _ := apm.StartSpan(ctx, "reconcile_certs", tracing.SpanTypeApp)
	defer span.End()

	results := reconciler.NewResult(ctx)
	selfSignedCert := ents.Spec.HTTP.TLS.SelfSignedCertificate
	if selfSignedCert != nil && selfSignedCert.Disabled {
		return results
	}

	labels := NewLabels(ents.Name)

	// reconcile CA certs first
	httpCa, err := certificates.ReconcileCAForOwner(
		driver.K8sClient(),
		driver.Scheme(),
		name.EntSearchNamer,
		ents,
		labels,
		certificates.HTTPCAType,
		caRotation,
	)
	if err != nil {
		return results.WithError(err)
	}

	// handle CA expiry via requeue
	results.WithResult(reconcile.Result{
		RequeueAfter: certificates.ShouldRotateIn(time.Now(), httpCa.Cert.NotAfter, caRotation.RotateBefore),
	})

	// discover and maybe reconcile for the http certificates to use
	httpCertificates, err := http.ReconcileHTTPCertificates(
		driver,
		ents,
		name.EntSearchNamer,
		httpCa,
		ents.Spec.HTTP.TLS,
		labels,
		services,
		certRotation,
	)
	if err != nil {
		return results.WithError(err)
	}
	primaryCert, err := certificates.GetPrimaryCertificate(httpCertificates.CertPem())
	if err != nil {
		results.WithError(err)
	}
	results.WithResult(reconcile.Result{
		RequeueAfter: certificates.ShouldRotateIn(time.Now(), primaryCert.NotAfter, certRotation.RotateBefore),
	})

	// reconcile http public cert secret
	results.WithError(http.ReconcileHTTPCertsPublicSecret(driver.K8sClient(), driver.Scheme(), ents, name.EntSearchNamer, httpCertificates))
	return results
}
