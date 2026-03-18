// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	apmv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/apm/v1"
	apmv1beta1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/apm/v1beta1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/beat/v1beta1"
	esv1beta1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1beta1"
	entv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/enterprisesearch/v1"
	entv1beta1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/enterprisesearch/v1beta1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	kbv1beta1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1beta1"
	emsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/maps/v1alpha1"
	eprv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	autoopsvalidation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/autoops/validation"
	esavalidation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/autoscaling/elasticsearch/validation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	commonwebhook "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook"
	esvalidation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/validation"
	lsvalidation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/logstash/validation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/webhook"
)

func chooseAndValidateIPFamily(ipFamilyStr string, ipFamilyDefault corev1.IPFamily) (corev1.IPFamily, error) {
	switch strings.ToLower(ipFamilyStr) {
	case "":
		return ipFamilyDefault, nil
	case "ipv4":
		return corev1.IPv4Protocol, nil
	case "ipv6":
		return corev1.IPv6Protocol, nil
	default:
		return ipFamilyDefault, fmt.Errorf("IP family can be one of: IPv4, IPv6 or \"\" to auto-detect, but was %s", ipFamilyStr)
	}
}

func validateCertExpirationFlags(validityFlag string, rotateBeforeFlag string) (time.Duration, time.Duration, error) {
	certValidity := viper.GetDuration(validityFlag)
	certRotateBefore := viper.GetDuration(rotateBeforeFlag)

	if certRotateBefore > certValidity {
		return certValidity, certRotateBefore, fmt.Errorf("%s must be larger than %s", validityFlag, rotateBeforeFlag)
	}

	return certValidity, certRotateBefore, nil
}

func setupWebhook(
	ctx context.Context,
	mgr manager.Manager,
	params operator.Parameters,
	webhookCertDir string,
	clientset kubernetes.Interface,
	exposedNodeLabels esvalidation.NodeLabels,
	managedNamespaces []string,
	tracer *apm.Tracer,
) {
	manageWebhookCerts := viper.GetBool(operator.ManageWebhookCertsFlag)
	if manageWebhookCerts {
		if err := reconcileWebhookCertsAndAddController(ctx, mgr, params.CertRotation, clientset, tracer); err != nil {
			log.Error(err, "unable to setup the webhook certificates")
			os.Exit(1)
		}
	}

	checker := commonlicense.NewLicenseChecker(mgr.GetClient(), params.OperatorNamespace)
	// setup webhooks for supported types
	commonwebhook.RegisterResourceWebhook(mgr, agentv1alpha1.WebhookPath, checker, managedNamespaces, agentv1alpha1.Validate, "Agent")
	commonwebhook.RegisterResourceWebhook(mgr, apmv1.WebhookPath, checker, managedNamespaces, apmv1.Validate, "APM Server")
	commonwebhook.RegisterResourceWebhook(mgr, apmv1beta1.WebhookPath, checker, managedNamespaces, apmv1beta1.Validate, "APM Server")
	commonwebhook.RegisterResourceWebhook(mgr, beatv1beta1.WebhookPath, checker, managedNamespaces, beatv1beta1.Validate, "Beat")
	commonwebhook.RegisterResourceWebhook(mgr, entv1.WebhookPath, checker, managedNamespaces, entv1.Validate, "Enterprise Search")
	commonwebhook.RegisterResourceWebhook(mgr, entv1beta1.WebhookPath, checker, managedNamespaces, entv1beta1.Validate, "Enterprise Search")
	commonwebhook.RegisterResourceWebhook(mgr, esv1beta1.WebhookPath, checker, managedNamespaces, esv1beta1.Validate, "Elasticsearch")
	commonwebhook.RegisterResourceWebhook(mgr, kbv1.WebhookPath, checker, managedNamespaces, kbv1.Validate, "Kibana")
	commonwebhook.RegisterResourceWebhook(mgr, kbv1beta1.WebhookPath, checker, managedNamespaces, kbv1beta1.Validate, "Kibana")
	commonwebhook.RegisterResourceWebhook(mgr, emsv1alpha1.WebhookPath, checker, managedNamespaces, emsv1alpha1.Validate, "Elastic Maps Server")
	commonwebhook.RegisterResourceWebhook(mgr, eprv1alpha1.WebhookPath, checker, managedNamespaces, eprv1alpha1.Validate, "Package Registry")
	commonwebhook.RegisterResourceWebhook(mgr, policyv1alpha1.WebhookPath, checker, managedNamespaces, policyv1alpha1.Validate, "Stack Config Policy")

	// Logstash, Elasticsearch, ElasticsearchAutoscaling, and AutoOps validating webhooks are wired up
	// differently in order to access the k8s client or license checker directly.
	esvalidation.RegisterWebhook(mgr, params.ValidateStorageClass, exposedNodeLabels, checker, managedNamespaces)
	esavalidation.RegisterWebhook(mgr, params.ValidateStorageClass, checker, managedNamespaces)
	lsvalidation.RegisterWebhook(mgr, params.ValidateStorageClass, managedNamespaces)
	autoopsvalidation.RegisterWebhook(mgr, checker, managedNamespaces)

	// wait for the secret to be populated in the local filesystem before returning
	interval := time.Second * 1
	timeout := time.Second * 30
	keyPath := filepath.Join(webhookCertDir, certificates.CertFileName)
	log.Info("Polling for the webhook certificate to be available", "path", keyPath)
	//nolint:staticcheck // keep
	err := wait.PollImmediateWithContext(ctx, interval, timeout, func(_ context.Context) (bool, error) {
		_, err := os.Stat(keyPath)
		// err could be that the file does not exist, but also that permission was denied or something else
		if os.IsNotExist(err) {
			log.V(1).Info("Webhook certificate file not present on filesystem yet", "path", keyPath)

			return false, nil
		} else if err != nil {
			log.Error(err, "Error checking if webhook secret path exists", "path", keyPath)
			return false, err
		}
		log.V(1).Info("Webhook certificate file present on filesystem", "path", keyPath)
		return true, nil
	})
	if err != nil {
		log.Error(err, "Timeout elapsed waiting for webhook certificate to be available", "path", keyPath, "timeout_seconds", timeout.Seconds())
		os.Exit(1)
	}
}

func reconcileWebhookCertsAndAddController(ctx context.Context, mgr manager.Manager, certRotation certificates.RotationParams, clientset kubernetes.Interface, tracer *apm.Tracer) error {
	ctx = tracing.NewContextTransaction(ctx, tracer, tracing.ReconciliationTxType, webhook.ControllerName, nil)
	defer tracing.EndContextTransaction(ctx)
	log.Info("Automatic management of the webhook certificates enabled")
	// Ensure that all the certificates needed by the webhook server are already created
	webhookParams := webhook.Params{
		Name:       viper.GetString(operator.WebhookNameFlag),
		Namespace:  viper.GetString(operator.OperatorNamespaceFlag),
		SecretName: viper.GetString(operator.WebhookSecretFlag),
		Rotation:   certRotation,
	}

	// retrieve the current webhook configuration interface
	wh, err := webhookParams.NewAdmissionControllerInterface(ctx, clientset)
	if err != nil {
		return err
	}

	// Force a first reconciliation to create the resources before the server is started
	if err := webhookParams.ReconcileResources(ctx, clientset, wh); err != nil {
		return err
	}

	return webhook.Add(mgr, webhookParams, clientset, wh, tracer)
}
