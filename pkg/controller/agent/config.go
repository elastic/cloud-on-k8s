// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"hash"
	"path"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

type connectionSettings struct {
	host, caFileName, version string
	credentials               association.Credentials
	caCerts                   []*x509.Certificate
}

func reconcileConfig(params Params, configHash hash.Hash) *reconciler.Results {
	defer tracing.Span(&params.Context)()
	results := reconciler.NewResult(params.Context)

	cfgBytes, err := buildConfig(params)
	if err != nil {
		return results.WithError(err)
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: params.Agent.Namespace,
			Name:      ConfigSecretName(params.Agent.Name),
			Labels:    labels.AddCredentialsLabel(params.Agent.GetIdentityLabels()),
		},
		Data: map[string][]byte{
			ConfigFileName: cfgBytes,
		},
	}

	if _, err = reconciler.ReconcileSecret(params.Context, params.Client, expected, &params.Agent); err != nil {
		return results.WithError(err)
	}

	_, _ = configHash.Write(cfgBytes)

	return results
}

func buildConfig(params Params) ([]byte, error) {
	cfg, err := buildOutputConfig(params)
	if err != nil {
		return nil, err
	}

	// get user config from `config` or `configRef`
	userConfig, err := getUserConfig(params)
	if err != nil {
		return nil, err
	}

	if err = cfg.MergeWith(userConfig); err != nil {
		return nil, err
	}

	return cfg.Render()
}

func buildOutputConfig(params Params) (*settings.CanonicalConfig, error) {
	if params.Agent.Spec.FleetModeEnabled() {
		// in fleet mode outputs are owned by fleet
		return settings.NewCanonicalConfig(), nil
	}

	allAssociations := params.Agent.GetAssociations()

	var esAssociations []commonv1.Association
	for _, assoc := range allAssociations {
		if assoc.AssociationType() == commonv1.ElasticsearchAssociationType {
			esAssociations = append(esAssociations, assoc)
		}
	}

	outputs := map[string]interface{}{}
	for i, assoc := range esAssociations {
		assocConf, err := assoc.AssociationConf()
		if err != nil {
			return settings.NewCanonicalConfig(), err
		}
		if !assocConf.IsConfigured() {
			return settings.NewCanonicalConfig(), nil
		}

		credentials, err := association.ElasticsearchAuthSettings(params.Context, params.Client, assoc)
		if err != nil {
			return settings.NewCanonicalConfig(), err
		}

		output := map[string]interface{}{
			"type":  "elasticsearch",
			"hosts": []string{assocConf.GetURL()},
		}

		if credentials.APIKey != "" {
			decodedAPIKey, err := base64.StdEncoding.DecodeString(credentials.APIKey)
			if err != nil {
				return settings.NewCanonicalConfig(), fmt.Errorf("error at decoding api-key from secret %s: %w", assocConf.AuthSecretName, err)
			}
			output["api_key"] = string(decodedAPIKey)
		} else {
			output["username"] = credentials.Username
			output["password"] = credentials.Password
		}
		if assocConf.GetCACertProvided() {
			output["ssl.certificate_authorities"] = []string{path.Join(certificatesDir(assoc), CAFileName)}
		}

		outputName := params.Agent.Spec.ElasticsearchRefs[i].OutputName
		if outputName == "" {
			if len(esAssociations) > 1 {
				return settings.NewCanonicalConfig(), errors.New("output is not named and there is more than one specified")
			}
			outputName = "default"
		}
		outputs[outputName] = output
	}

	return settings.NewCanonicalConfigFrom(map[string]interface{}{
		"outputs": outputs,
	})
}

// getUserConfig extracts the config either from the spec `config` field or from the Secret referenced by spec
// `configRef` field.
func getUserConfig(params Params) (*settings.CanonicalConfig, error) {
	if params.Agent.Spec.Config != nil {
		return settings.NewCanonicalConfigFrom(params.Agent.Spec.Config.Data)
	}
	return common.ParseConfigRef(params, &params.Agent, params.Agent.Spec.ConfigRef, ConfigFileName)
}

// extractPodConnectionSettings extracts connections settings to be used inside an Elastic Agent Pod. That is without
// certificates which are mounted directly into the Pod, instead the connection settings contain a path which points to
// the future location of the certificates in the Pod.
func extractPodConnectionSettings(
	ctx context.Context,
	agent agentv1alpha1.Agent,
	client k8s.Client,
	associationType commonv1.AssociationType,
) (connectionSettings, *commonv1.AssociationConf, error) {
	assoc, err := association.SingleAssociationOfType(agent.GetAssociations(), associationType)
	if err != nil {
		return connectionSettings{}, nil, err
	}

	if assoc == nil {
		errTemplate := "association of type %s not found in %d associations"
		return connectionSettings{}, nil, fmt.Errorf(errTemplate, associationType, len(agent.GetAssociations()))
	}

	credentials, err := association.ElasticsearchAuthSettings(ctx, client, assoc)
	if err != nil {
		return connectionSettings{}, nil, err
	}

	assocConf, err := assoc.AssociationConf()
	if err != nil {
		return connectionSettings{}, nil, err
	}

	ca := ""
	if assocConf.GetCACertProvided() {
		ca = path.Join(certificatesDir(assoc), CAFileName)
	}

	return connectionSettings{
		host:        assocConf.GetURL(),
		caFileName:  ca,
		credentials: credentials,
		version:     assocConf.Version,
	}, assocConf, err
}

// extractClientConnectionSettings same as extractPodConnectionSettings but for use inside the operator or any other
// client that needs direct access to the relevant CA certificates of the associated object (if TLS is configured)
func extractClientConnectionSettings(
	ctx context.Context,
	agent agentv1alpha1.Agent,
	client k8s.Client,
	associationType commonv1.AssociationType,
) (connectionSettings, error) {
	settings, assocConf, err := extractPodConnectionSettings(ctx, agent, client, associationType)
	if err != nil {
		return connectionSettings{}, err
	}
	if !assocConf.GetCACertProvided() {
		return settings, nil
	}
	var caSecret corev1.Secret
	if err := client.Get(ctx, types.NamespacedName{Name: assocConf.GetCASecretName(), Namespace: agent.Namespace}, &caSecret); err != nil {
		return connectionSettings{}, err
	}
	bytes, ok := caSecret.Data[CAFileName]
	if !ok {
		return connectionSettings{}, fmt.Errorf("no %s in %s", CAFileName, k8s.ExtractNamespacedName(&caSecret))
	}
	certs, err := certificates.ParsePEMCerts(bytes)
	if err != nil {
		return connectionSettings{}, err
	}
	settings.caCerts = certs
	return settings, nil
}
