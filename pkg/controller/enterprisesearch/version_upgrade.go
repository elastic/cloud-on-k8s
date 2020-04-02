// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	entsname "github.com/elastic/cloud-on-k8s/pkg/controller/enterprisesearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

const (
	// ReadOnlyModeAnnotationName stores "true" when read-only mode is enabled.
	ReadOnlyModeAnnotationName = "enterprisesearch.k8s.elastic.co/read-only"
	// ReadOnlyModeAPIPath is the HTTP path of the read-only mode API.
	ReadOnlyModeAPIPath = "/api/ent/v1/internal/read_only_mode"
	// ReadOnlyModeReqTimeout is the duration after which a request to the read-only mode API should be canceled.
	ReadOnlyModeReqTimeout = 1 * time.Minute
)

// VersionUpgrade toggles read-only mode on Enterprise Search during version upgrades.
type VersionUpgrade struct {
	k8sClient  k8s.Client
	recorder   record.EventRecorder
	ents       entsv1beta1.EnterpriseSearch
	dialer     net.Dialer   // optional custom dialer for the http client
	httpClient *http.Client // custom http client, will be created if nil
}

// Handle Enterprise Search version upgrades if necessary, by toggling read-only mode.
func (r *VersionUpgrade) Handle(ctx context.Context) error {
	expectedVersion, err := version.Parse(r.ents.Spec.Version)
	if err != nil {
		return err
	}

	upgradeRequested, err := r.isVersionUpgrade(*expectedVersion)
	if err != nil {
		return err
	}

	if upgradeRequested && !r.ents.AssociationConf().AuthIsConfigured() {
		// A version upgrade is scheduled, but we don't know how to reach the Enterprise Search API
		// since we don't have any Elasticsearch user available.
		// Move on with the upgrade: this will cause the Pod in the new version to crash at startup with explicit logs.
		msg := "Detected version upgrade with no association to Elasticsearch, " +
			"please toggle read-only mode manually, otherwise the new version will crash at startup."
		log.Info(msg, "namespace", r.ents.Namespace, "ents_name", r.ents.Name)
		r.recorder.Event(&r.ents, corev1.EventTypeWarning, events.EventReasonUpgraded, msg)
		return nil
	}

	actualPods, err := r.getActualPods()
	if err != nil {
		return err
	}

	if upgradeRequested {
		if len(actualPods) == 0 {
			msg := "a version upgrade is scheduled, but no Pod in the prior version is running:" +
				"waiting for at least one Pod in the prior version to be running in order to enable read-only mode"
			log.Info(msg, "namespace", r.ents.Namespace, "ents_name", r.ents.Name)
			r.recorder.Event(&r.ents, corev1.EventTypeWarning, events.EventReasonDelayed, msg)
			// surface this as an error, since rather unexpected, and abort reconciliation
			return errors.New(msg)
		}
		// enable read-only mode before moving on with the deployment upgrade
		return r.enableReadOnlyMode(ctx)
	}

	// if the old version is still running, we cannot disable read-only mode yet
	// we'll retry eventually once pod rotation is over
	if oldVersionStillRunning, err := r.isPriorVersionStillRunning(*expectedVersion); err != nil || oldVersionStillRunning {
		return err
	}

	return r.disableReadOnlyMode(ctx)
}

// enableReadOnlyMode enables read-only mode through an API call, if not already done,
// and stores the read-only mode state in an annotation on the Enterprise Search resource.
func (r *VersionUpgrade) enableReadOnlyMode(ctx context.Context) error {
	if hasReadOnlyAnnotationTrue(r.ents) {
		// nothing to do, already done
		return nil
	}

	log.Info("Enabling read-only mode for version upgrade",
		"namespace", r.ents.Namespace, "ents_name", r.ents.Name, "target_version", r.ents.Spec.Version)

	// call the Enterprise Search API
	if err := r.setReadOnlyMode(ctx, true); err != nil {
		return err
	}

	// annotate the resource to avoid doing the same API call over and over again
	// (in practice, it may happen again if the next reconciliation does not have an up-to-date cache)
	if r.ents.Annotations == nil {
		r.ents.Annotations = map[string]string{}
	}
	r.ents.Annotations[ReadOnlyModeAnnotationName] = "true"
	return r.k8sClient.Update(&r.ents)
}

// disableReadOnlyMode disables read-only mode through an API call, if enabled previously,
// and removes the read-only mode annotation.
func (r *VersionUpgrade) disableReadOnlyMode(ctx context.Context) error {
	if !hasReadOnlyAnnotationTrue(r.ents) {
		// nothing to do, read-only was not set
		return nil
	}

	log.Info("Disabling read-only mode",
		"namespace", r.ents.Namespace, "ents_name", r.ents.Name)

	// call the Enterprise Search API
	if err := r.setReadOnlyMode(ctx, false); err != nil {
		return err
	}

	// remove the annotation to avoid doing the same API call over and over again
	// (in practice, it may happen again if the next reconciliation does not have an up-to-date cache)
	delete(r.ents.Annotations, ReadOnlyModeAnnotationName)
	return r.k8sClient.Update(&r.ents)
}

// hasReadOnlyAnnotationTrue returns true if the read-only mode annotation is set to true,
// and false otherwise.
func hasReadOnlyAnnotationTrue(ents entsv1beta1.EnterpriseSearch) bool {
	value, exists := ents.Annotations[ReadOnlyModeAnnotationName]
	return exists && value == "true"
}

// setReadOnlyMode performs an API call to Enterprise Search to set the read-only mode setting to the given value.
func (r *VersionUpgrade) setReadOnlyMode(ctx context.Context, enabled bool) error {
	httpClient := r.httpClient
	if httpClient == nil {
		// build an HTTP client to reach the Enterprise Search service
		tlsCerts, err := r.retrieveTLSCerts()
		if err != nil {
			return err
		}
		httpClient = common.HTTPClient(r.dialer, tlsCerts)
		defer httpClient.CloseIdleConnections()
	}

	request, err := r.readOnlyModeRequest(enabled)
	if err != nil {
		return err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, ReadOnlyModeReqTimeout)
	defer cancel()
	request = request.WithContext(timeoutCtx)

	resp, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("invalid read-only mode API response (status code %d): %s", resp.StatusCode, respBody)
	}

	return nil
}

// serviceURL builds the URL of the Enterprise Search service.
func (r *VersionUpgrade) serviceURL() string {
	return fmt.Sprintf("%s://%s.%s.svc:%d",
		r.ents.Spec.HTTP.Protocol(), entsname.HTTPService(r.ents.Name), r.ents.Namespace, HTTPPort)
}

// readOnlyModeRequest builds the HTTP request to toggle the read-only mode on Enterprise Search.
func (r *VersionUpgrade) readOnlyModeRequest(enabled bool) (*http.Request, error) {
	username, password, err := association.ElasticsearchAuthSettings(r.k8sClient, &r.ents)
	if err != nil {
		return nil, err
	}

	url := stringsutil.Concat(r.serviceURL(), ReadOnlyModeAPIPath)

	body := bytes.NewBuffer([]byte(fmt.Sprintf("{\"enabled\": %t}", enabled)))

	req, err := http.NewRequest(http.MethodPut, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.SetBasicAuth(username, password)

	return req, nil
}

// isVersionUpgrade returns true if the existing Deployment specifies a version prior to the one
// specified in the EnterpriseSearch resource.
func (r *VersionUpgrade) isVersionUpgrade(expectedVersion version.Version) (bool, error) {
	var deployment appsv1.Deployment
	nsn := types.NamespacedName{Name: entsname.Deployment(r.ents.Name), Namespace: r.ents.Namespace}
	err := r.k8sClient.Get(nsn, &deployment)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// first deployment, not an upgrade
			return false, nil
		}
		return false, err
	}

	podVersion, err := version.FromLabels(deployment.Spec.Template.Labels, VersionLabelName)
	if err != nil {
		return false, err
	}
	return expectedVersion.IsAfter(*podVersion), nil
}

// isPriorVersionStillRunning returns true if at least one Pod runs a version prior to the expected one.
func (r *VersionUpgrade) isPriorVersionStillRunning(expectedVersion version.Version) (bool, error) {
	pods, err := r.getActualPods()
	if err != nil {
		return false, err
	}
	for _, p := range pods {
		podVersion, err := version.FromLabels(p.Labels, VersionLabelName)
		if err != nil {
			return false, err
		}
		if expectedVersion.IsAfter(*podVersion) {
			return true, nil
		}
	}
	return false, nil
}

// getActualPods returns all existing Pods for this Enterprise Search resource.
func (r *VersionUpgrade) getActualPods() ([]corev1.Pod, error) {
	var pods corev1.PodList
	ns := client.InNamespace(r.ents.Namespace)
	if err := r.k8sClient.List(&pods, client.MatchingLabels(Labels(r.ents.Name)), ns); err != nil {
		return nil, err
	}
	return pods.Items, nil
}

// retrieveTLSCerts returns the TLS certs used by Enterprise Search.
func (r *VersionUpgrade) retrieveTLSCerts() ([]*x509.Certificate, error) {
	var certsSecret corev1.Secret
	nsn := types.NamespacedName{
		Namespace: r.ents.Namespace,
		Name:      certificates.InternalCertsSecretName(entsname.EntSearchNamer, r.ents.Name),
	}
	if err := r.k8sClient.Get(nsn, &certsSecret); err != nil {
		return nil, err
	}
	certData, exists := certsSecret.Data[certificates.CertFileName]
	if !exists {
		return nil, fmt.Errorf("no %s found in secret %s", certificates.CertFileName, certsSecret.Name)
	}
	return certificates.ParsePEMCerts(certData)
}
