// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"reflect"
	"strings"

	"github.com/elastic/k8s-operators/operators/pkg/apis"
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/reconciler"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	licenseFileFlag       = "license-file"
	operatorNamespaceFlag = "operator-namespace"
	eulaAcceptanceFlag    = "accept-eula"
)

var (
	Cmd = &cobra.Command{
		Use:   "install-license",
		Short: "Installs an existing enterprise license",
		Long: "Installs an existing enterprise license into the cluster by transforming it into the expected CRD" +
			" and applying it.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// set up the k8s client
			kubecfg := config.GetConfigOrDie()
			if err := apis.AddToScheme(scheme.Scheme); err != nil {
				return err
			}
			k8sClient, err := client.New(kubecfg, client.Options{Scheme: scheme.Scheme})
			if err != nil {
				return err
			}
			// create params for the command and validate them
			params := Params{
				OperatorNs:  viper.GetString(operatorNamespaceFlag),
				LicenseFile: viper.GetString(licenseFileFlag),
				Eula:        viper.GetBool(eulaAcceptanceFlag),
				Client:      k8sClient,
			}
			if err := params.Validate(); err != nil {
				return err
			}
			// the actual command
			return extractTransformLoadLicense(params)
		}}
	log = logf.Log.WithName("license-installer")
)

func handleError(err error) {
	if err != nil {
		log.Error(err, "Fatal error")
		os.Exit(1)
	}
}

func init() {
	Cmd.Flags().String(
		licenseFileFlag,
		"",
		"original license file as obtained from Elastic",
	)
	Cmd.Flags().String(
		operatorNamespaceFlag,
		"",
		"namespace the operator is running in",
	)
	Cmd.Flags().Bool(
		eulaAcceptanceFlag,
		false,
		"indicate that you have read and accepted the EULA at https://www.elastic.co/eula",
	)

	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	handleError(viper.BindPFlags(Cmd.Flags()))
	viper.AutomaticEnv()
}

type Params struct {
	OperatorNs  string
	LicenseFile string
	Eula        bool
	Client      client.Client
}

func (p Params) Validate() error {
	for _, vs := range [][]string{
		{p.LicenseFile, licenseFileFlag},
		{p.OperatorNs, operatorNamespaceFlag},
	} {
		if vs[0] == "" {
			return fmt.Errorf("--%s is a required parameter", vs[1])
		}
	}
	return nil
}

func extractTransformLoadLicense(p Params) error {
	var source SourceEnterpriseLicense
	fileBytes, err := ioutil.ReadFile(p.LicenseFile)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(fileBytes, &source); err != nil {
		return err
	}

	name := licenseResourceName(source)
	secretName := name + "-license-sigs"

	const enterpriseSig = "enterprise-license-sig"
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: p.OperatorNs,
		},
		Data: map[string][]byte{
			enterpriseSig: []byte(source.Data.Signature),
		},
	}

	var clusterLicenses []v1alpha1.ClusterLicenseSpec
	for _, l := range source.Data.ClusterLicenses {
		license := l.License
		sigKey := license.UID[24:]
		secret.Data[sigKey] = []byte(license.Signature)

		clusterLicenses = append(clusterLicenses,
			v1alpha1.ClusterLicenseSpec{
				LicenseMeta: v1alpha1.LicenseMeta{
					UID:                license.UID,
					IssueDateInMillis:  license.IssueDateInMillis,
					ExpiryDateInMillis: license.ExpiryDateInMillis,
					IssuedTo:           license.IssuedTo,
					Issuer:             license.Issuer,
					StartDateInMillis:  license.StartDateInMillis,
				},
				MaxNodes: license.MaxNodes,
				Type:     v1alpha1.LicenseType(license.Type),
				SignatureRef: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
					Key: sigKey,
				},
			})
	}

	var reconciledSecret corev1.Secret
	if err := reconciler.ReconcileResource(reconciler.Params{
		Client:     k8s.WrapClient(p.Client),
		Scheme:     scheme.Scheme,
		Owner:      nil,
		Expected:   &secret,
		Reconciled: &reconciledSecret,
		NeedsUpdate: func() bool {
			return !reflect.DeepEqual(secret.Data, reconciledSecret.Data)
		},
		UpdateReconciled: func() {
			reconciledSecret.Data = secret.Data
		},
	}); err != nil {
		return err
	}

	expectedLicense := v1alpha1.EnterpriseLicense{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: p.OperatorNs,
		},
		Spec: v1alpha1.EnterpriseLicenseSpec{
			LicenseMeta: v1alpha1.LicenseMeta{
				UID:                source.Data.UID,
				IssueDateInMillis:  source.Data.IssueDateInMillis,
				ExpiryDateInMillis: source.Data.ExpiryDateInMillis,
				IssuedTo:           source.Data.IssuedTo,
				Issuer:             source.Data.Issuer,
				StartDateInMillis:  source.Data.StartDateInMillis,
			},
			Type:         v1alpha1.EnterpriseLicenseType(source.Data.Type),
			MaxInstances: source.Data.MaxInstances,
			SignatureRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key: enterpriseSig,
			},
			ClusterLicenseSpecs: clusterLicenses,
			Eula: v1alpha1.EulaState{
				Accepted: p.Eula,
			},
		},
	}
	var reconciledLicense v1alpha1.EnterpriseLicense
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     k8s.WrapClient(p.Client),
		Scheme:     scheme.Scheme,
		Expected:   &expectedLicense,
		Reconciled: &reconciledLicense,
		NeedsUpdate: func() bool {
			return !reflect.DeepEqual(reconciledLicense.Spec, expectedLicense.Spec)
		},
		UpdateReconciled: func() {
			reconciledLicense.Spec = expectedLicense.Spec
		},
	})
}

func licenseResourceName(source SourceEnterpriseLicense) string {
	limit := math.Min(64, float64(len(source.Data.IssuedTo)))
	return strings.ToLower(strings.Replace(source.Data.IssuedTo[:int(limit)], " ", "", -1)) + "-" + source.Data.UID[24:]
}
