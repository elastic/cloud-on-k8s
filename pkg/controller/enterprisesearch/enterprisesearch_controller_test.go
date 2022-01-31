// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package enterprisesearch

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	entv1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func TestReconcileEnterpriseSearch_Reconcile_Unmanaged(t *testing.T) {
	// unmanaged resource, should do nothing
	sample := entv1.EnterpriseSearch{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sample", Annotations: map[string]string{
			common.ManagedAnnotation: "false",
		}},
		Spec: entv1.EnterpriseSearchSpec{Version: "7.7.0"},
	}
	r := &ReconcileEnterpriseSearch{
		Client: k8s.NewFakeClient(&sample),
	}
	result, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "sample", Namespace: "ns"}})
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, result)
}

func TestReconcileEnterpriseSearch_Reconcile_NotFound(t *testing.T) {
	// resource not found, should clear watches
	r := &ReconcileEnterpriseSearch{
		Client:         k8s.NewFakeClient(),
		dynamicWatches: watches.NewDynamicWatches(),
	}
	// simulate existing watches
	nsn := types.NamespacedName{Name: "sample", Namespace: "ns"}
	require.NoError(t, watches.WatchUserProvidedSecrets(nsn, r.DynamicWatches(), common.ConfigRefWatchName(nsn), []string{"watched-secret"}))
	// simulate a custom http tls secret
	require.NoError(t, watches.WatchUserProvidedSecrets(nsn, r.dynamicWatches, "sample-ent-http-certificate", []string{"user-tls-secret"}))
	require.NotEmpty(t, r.dynamicWatches.Secrets.Registrations())

	result, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: nsn})
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, result)

	// watch should have been cleared out
	require.Empty(t, r.dynamicWatches.Secrets.Registrations())
}

func TestReconcileEnterpriseSearch_Reconcile_AssociationNotConfigured(t *testing.T) {
	// an Elasticsearch ref is specified, but its configuration is not set: should do nothing
	sample := entv1.EnterpriseSearch{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sample"},
		Spec: entv1.EnterpriseSearchSpec{
			Version:          "7.7.0",
			ElasticsearchRef: commonv1.ObjectSelector{Namespace: "ns", Name: "es"},
		},
	}
	fakeRecorder := record.NewFakeRecorder(10)
	r := &ReconcileEnterpriseSearch{
		Client:         k8s.NewFakeClient(&sample),
		dynamicWatches: watches.NewDynamicWatches(),
		recorder:       fakeRecorder,
	}
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "sample", Namespace: "ns"}})
	require.NoError(t, err)
	// should just requeue until the resource is updated
	require.Equal(t, reconcile.Result{}, res)
	// an event should be emitted
	e := <-fakeRecorder.Events
	require.Equal(t, "Warning AssociationError Association backend for elasticsearch is not configured", e)
}

func TestReconcileEnterpriseSearch_Reconcile_InvalidResource(t *testing.T) {
	// spec.Version missing from the spec
	sample := entv1.EnterpriseSearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sample"}}
	fakeRecorder := record.NewFakeRecorder(10)
	r := &ReconcileEnterpriseSearch{
		Client:   k8s.NewFakeClient(&sample),
		recorder: fakeRecorder,
	}
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "sample", Namespace: "ns"}})
	// should return an error
	require.Error(t, err)
	require.Contains(t, err.Error(), "spec.version: Invalid value")
	require.Equal(t, reconcile.Result{}, res)
	// an event should be emitted
	e := <-fakeRecorder.Events
	require.Contains(t, e, "spec.version: Invalid value")
}

func TestReconcileEnterpriseSearch_Reconcile_Create_Update_Resources(t *testing.T) {
	sample := entv1.EnterpriseSearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sample"},
		Spec: entv1.EnterpriseSearchSpec{
			Version: "7.7.0",
			Count:   3,
		}}
	r := &ReconcileEnterpriseSearch{
		Client:         k8s.NewFakeClient(&sample),
		dynamicWatches: watches.NewDynamicWatches(),
		recorder:       record.NewFakeRecorder(10),
		Parameters:     operator.Parameters{OperatorInfo: about.OperatorInfo{BuildInfo: about.BuildInfo{Version: "1.0.0"}}},
	}

	checkResources := func() {
		// should create a service
		var service corev1.Service
		err := r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: HTTPServiceName(sample.Name)}, &service)
		require.NoError(t, err)
		require.Equal(t, int32(3002), service.Spec.Ports[0].Port)

		// should create internal ca, internal http certs secret, public http certs secret
		var caSecret corev1.Secret
		err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "sample-ent-http-ca-internal"}, &caSecret)
		require.NoError(t, err)
		require.NotEmpty(t, caSecret.Data)

		var httpInternalSecret corev1.Secret
		err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "sample-ent-http-certs-internal"}, &httpInternalSecret)
		require.NoError(t, err)
		require.NotEmpty(t, httpInternalSecret.Data)

		var httpPublicSecret corev1.Secret
		err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "sample-ent-http-certs-public"}, &httpPublicSecret)
		require.NoError(t, err)
		require.NotEmpty(t, httpPublicSecret.Data)

		// should create a secret for the configuration
		var config corev1.Secret
		err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "sample-ent-config"}, &config)
		require.NoError(t, err)
		require.Contains(t, string(config.Data["enterprise-search.yml"]), "external_url:")

		// should create a 3-replicas deployment
		var dep appsv1.Deployment
		err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "sample-ent"}, &dep)
		require.NoError(t, err)
		require.True(t, *dep.Spec.Replicas == 3)
		// with the config hash annotation set
		require.NotEmpty(t, dep.Spec.Template.Annotations[ConfigHashAnnotationName])
	}

	// first call
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "sample", Namespace: "ns"}})
	require.NoError(t, err)
	// should requeue for cert expiration
	require.NotZero(t, res.RequeueAfter)
	// all resources should be created
	checkResources()

	// call-again: no-op
	res, err = r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "sample", Namespace: "ns"}})
	require.NoError(t, err)
	require.NotZero(t, res.RequeueAfter)
	// all resources should be the same
	checkResources()

	// modify the deployment: 2 replicas instead of 3
	var dep appsv1.Deployment
	err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "sample-ent"}, &dep)
	require.NoError(t, err)
	replicas := int32(2)
	dep.Spec.Replicas = &replicas
	err = r.Client.Update(context.Background(), &dep)
	require.NoError(t, err)
	// delete the http service
	var service corev1.Service
	err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: HTTPServiceName(sample.Name)}, &service)
	require.NoError(t, err)
	err = r.Client.Delete(context.Background(), &service)
	require.NoError(t, err)
	// delete the configuration secret entry
	var config corev1.Secret
	err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "sample-ent-config"}, &config)
	require.NoError(t, err)
	config.Data = nil
	err = r.Client.Update(context.Background(), &config)
	require.NoError(t, err)
	// delete the http certs data
	var httpInternalSecret corev1.Secret
	err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "sample-ent-http-certs-internal"}, &httpInternalSecret)
	require.NoError(t, err)
	httpInternalSecret.Data = nil
	err = r.Client.Update(context.Background(), &httpInternalSecret)
	require.NoError(t, err)

	// call again: all resources should be updated to revert our manual changes above
	res, err = r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "sample", Namespace: "ns"}})
	require.NoError(t, err)
	require.NotZero(t, res.RequeueAfter)
	// all resources should be the same
	checkResources()
}

func TestReconcileEnterpriseSearch_doReconcile_AssociationDelaysVersionUpgrade(t *testing.T) {
	// associate Enterprise Search 7.7.0 to Elasticsearch 7.7.0
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "some-es"}}
	ent := entv1.EnterpriseSearch{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ent"},
		Spec:       entv1.EnterpriseSearchSpec{Version: "7.7.0", ElasticsearchRef: commonv1.ObjectSelector{Name: "some-es"}}}
	esTLSCertsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: ent.Namespace, Name: "es-tls-certs"},
		Data: map[string][]byte{
			certificates.CertFileName: []byte("es-cert-data"),
		},
	}
	assocConf := commonv1.AssociationConf{
		Version:        "7.7.0",
		AuthSecretName: "ent-user",
		CASecretName:   "es-tls-certs",
		URL:            "https://elasticsearch-sample-es-http.default.svc:9200"}
	// the association conf is required to reconcile the Enterprise Search resource, we set it manually because it is not
	// persisted and instead set by the association controller in real condition
	ent.SetAssociationConf(&assocConf)

	r := &ReconcileEnterpriseSearch{
		Client:         k8s.NewFakeClient(&ent, &es, &esTLSCertsSecret),
		dynamicWatches: watches.NewDynamicWatches(),
		recorder:       record.NewFakeRecorder(10),
		Parameters:     operator.Parameters{OperatorInfo: about.OperatorInfo{BuildInfo: about.BuildInfo{Version: "1.0.0"}}},
	}
	_, err := r.doReconcile(context.Background(), ent)
	require.NoError(t, err)

	// the Enterprise Search deployment should be created and specify version 7.7.0
	var dep appsv1.Deployment
	err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: DeploymentName(ent.Name)}, &dep)
	require.NoError(t, err)
	require.Equal(t, "7.7.0", dep.Spec.Template.Labels[VersionLabelName])
	// retrieve the updated ent resource
	require.NoError(t, r.Client.Get(context.Background(), k8s.ExtractNamespacedName(&ent), &ent))

	// update EnterpriseSearch to 7.8.0: the deployment should stay in version 7.7.0 since
	// Elasticsearch still runs 7.7.0
	ent.Spec.Version = "7.8.0"
	err = r.Client.Update(context.Background(), &ent)
	require.NoError(t, err)
	ent.SetAssociationConf(&assocConf) // required to reconcile, normally set by the assoc controller
	_, err = r.doReconcile(context.Background(), ent)
	require.NoError(t, err)
	err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: DeploymentName(ent.Name)}, &dep)
	require.NoError(t, err)
	require.Equal(t, "7.7.0", dep.Spec.Template.Labels[VersionLabelName])

	// retrieve the updated ent resource
	require.NoError(t, r.Client.Get(context.Background(), k8s.ExtractNamespacedName(&ent), &ent))
	// update the associated Elasticsearch to 7.8.0: Enterprise Search should now be upgraded to 7.8.0
	assocConf.Version = "7.8.0"
	ent.SetAssociationConf(&assocConf) // required to reconcile, normally set by the assoc controller
	_, err = r.doReconcile(context.Background(), ent)
	require.NoError(t, err)

	err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: DeploymentName(ent.Name)}, &dep)
	require.NoError(t, err)
	require.Equal(t, "7.8.0", dep.Spec.Template.Labels[VersionLabelName])
	// retrieve the updated ent resource
	require.NoError(t, r.Client.Get(context.Background(), k8s.ExtractNamespacedName(&ent), &ent))

	// update EnterpriseSearch to 7.8.1: this should be allowed even though Elasticsearch
	// is running 7.8.0 (different patch versions)
	ent.Spec.Version = "7.8.1"
	err = r.Client.Update(context.Background(), &ent)
	require.NoError(t, err)

	ent.SetAssociationConf(&assocConf) // required to reconcile, normally set by the assoc controller
	_, err = r.doReconcile(context.Background(), ent)
	require.NoError(t, err)

	err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: DeploymentName(ent.Name)}, &dep)
	require.NoError(t, err)
	require.Equal(t, "7.8.1", dep.Spec.Template.Labels[VersionLabelName])
}

type fakeClientStatusCall struct {
	updateCalled bool
	k8s.Client
}

func (f *fakeClientStatusCall) Status() client.StatusWriter {
	f.updateCalled = true // Status() has been requested for update
	return f.Client.Status()
}

func TestReconcileEnterpriseSearch_updateStatus(t *testing.T) {
	tests := []struct {
		name                   string
		ent                    entv1.EnterpriseSearch
		deploy                 appsv1.Deployment
		svcName                string
		wantStatus             entv1.EnterpriseSearchStatus
		wantEvent              bool
		wantStatusUpdateCalled bool
	}{
		{
			name: "happy path",
			ent:  entv1.EnterpriseSearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ent"}},
			deploy: appsv1.Deployment{Status: appsv1.DeploymentStatus{
				AvailableReplicas: 3,
				Conditions: []appsv1.DeploymentCondition{
					{
						Type:   appsv1.DeploymentAvailable,
						Status: corev1.ConditionTrue,
					},
				},
			}},
			svcName: "http-service",
			wantStatus: entv1.EnterpriseSearchStatus{
				DeploymentStatus: commonv1.DeploymentStatus{
					AvailableNodes: 3,
					Version:        "",
					Health:         "green",
				},
				ExternalService: "http-service",
			},
			wantStatusUpdateCalled: true,
		},
		{
			name: "preserve existing association status",
			ent: entv1.EnterpriseSearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ent"},
				Status: entv1.EnterpriseSearchStatus{Association: commonv1.AssociationEstablished}},
			deploy: appsv1.Deployment{Status: appsv1.DeploymentStatus{
				AvailableReplicas: 3,
				Conditions: []appsv1.DeploymentCondition{
					{
						Type:   appsv1.DeploymentAvailable,
						Status: corev1.ConditionTrue,
					},
				},
			}},
			svcName: "http-service",
			wantStatus: entv1.EnterpriseSearchStatus{
				DeploymentStatus: commonv1.DeploymentStatus{
					AvailableNodes: 3,
					Version:        "",
					Health:         "green",
				},
				ExternalService: "http-service",
				Association:     commonv1.AssociationEstablished,
			},
			wantStatusUpdateCalled: true,
		},
		{
			name: "red health if deployment not available",
			ent:  entv1.EnterpriseSearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ent"}},
			deploy: appsv1.Deployment{Status: appsv1.DeploymentStatus{
				AvailableReplicas: 3,
				Conditions: []appsv1.DeploymentCondition{
					{
						Type:   appsv1.DeploymentAvailable,
						Status: corev1.ConditionFalse,
					},
				},
			}},
			svcName: "http-service",
			wantStatus: entv1.EnterpriseSearchStatus{
				DeploymentStatus: commonv1.DeploymentStatus{
					AvailableNodes: 3,
					Version:        "",
					Health:         "red",
				},
				ExternalService: "http-service",
			},
			wantStatusUpdateCalled: true,
		},
		{
			name: "update existing status when replicas count changes",
			ent: entv1.EnterpriseSearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ent"},
				Status: entv1.EnterpriseSearchStatus{
					DeploymentStatus: commonv1.DeploymentStatus{
						AvailableNodes: 3,
						Version:        "",
						Health:         "green",
					},
					ExternalService: "http-service",
				}},
			deploy: appsv1.Deployment{Status: appsv1.DeploymentStatus{
				AvailableReplicas: 4,
				Conditions: []appsv1.DeploymentCondition{
					{
						Type:   appsv1.DeploymentAvailable,
						Status: corev1.ConditionTrue,
					},
				},
			}},
			svcName: "http-service",
			wantStatus: entv1.EnterpriseSearchStatus{
				DeploymentStatus: commonv1.DeploymentStatus{
					AvailableNodes: 4,
					Version:        "",
					Health:         "green",
				},
				ExternalService: "http-service",
			},
			wantStatusUpdateCalled: true,
		},
		{
			name: "don't do a status update if not necessary",
			ent: entv1.EnterpriseSearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ent"},
				Status: entv1.EnterpriseSearchStatus{
					DeploymentStatus: commonv1.DeploymentStatus{
						AvailableNodes: 3,
						Version:        "",
						Health:         "green",
					},
					ExternalService: "http-service",
				}},
			deploy: appsv1.Deployment{Status: appsv1.DeploymentStatus{
				AvailableReplicas: 3,
				Conditions: []appsv1.DeploymentCondition{
					{
						Type:   appsv1.DeploymentAvailable,
						Status: corev1.ConditionTrue,
					},
				},
			}},
			svcName: "http-service",
			wantStatus: entv1.EnterpriseSearchStatus{
				DeploymentStatus: commonv1.DeploymentStatus{
					AvailableNodes: 3,
					Version:        "",
					Health:         "green",
				},
				ExternalService: "http-service",
			},
			wantStatusUpdateCalled: false,
		},
		{
			name: "emit an event when health goes from green to red",
			ent: entv1.EnterpriseSearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ent"},
				Status: entv1.EnterpriseSearchStatus{
					DeploymentStatus: commonv1.DeploymentStatus{
						AvailableNodes: 3,
						Version:        "",
						Health:         "green",
					},
					ExternalService: "http-service",
				}},
			deploy: appsv1.Deployment{Status: appsv1.DeploymentStatus{
				AvailableReplicas: 3,
				Conditions: []appsv1.DeploymentCondition{
					{
						Type:   appsv1.DeploymentAvailable,
						Status: corev1.ConditionFalse,
					},
				},
			}},
			svcName: "http-service",
			wantStatus: entv1.EnterpriseSearchStatus{
				DeploymentStatus: commonv1.DeploymentStatus{
					AvailableNodes: 3,
					Version:        "",
					Health:         "red",
				},
				ExternalService: "http-service",
			},
			wantEvent:              true,
			wantStatusUpdateCalled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &fakeClientStatusCall{Client: k8s.NewFakeClient(&tt.ent)}
			fakeRecorder := record.NewFakeRecorder(10)
			r := &ReconcileEnterpriseSearch{
				Client:   c,
				recorder: fakeRecorder,
			}
			err := r.updateStatus(tt.ent, tt.deploy, tt.svcName)
			require.NoError(t, err)

			require.Equal(t, tt.wantStatusUpdateCalled, c.updateCalled)

			var updatedEnt entv1.EnterpriseSearch
			err = c.Get(context.Background(), k8s.ExtractNamespacedName(&tt.ent), &updatedEnt)
			require.NoError(t, err)
			require.Equal(t, tt.wantStatus, updatedEnt.Status)

			if tt.wantEvent {
				<-fakeRecorder.Events
			} else {
				// no event expected
				select {
				case e := <-fakeRecorder.Events:
					require.Fail(t, "no event expected but got one", "event", e)
				default:
					// ok
				}
			}
		})
	}
}

func Test_buildConfigHash(t *testing.T) {
	ent := entv1.EnterpriseSearch{ObjectMeta: metav1.ObjectMeta{
		Namespace: "ns", Name: "ent"}}

	entWithAssociation := *ent.DeepCopy()
	esTLSCertsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: ent.Namespace, Name: "es-tls-certs"},
		Data: map[string][]byte{
			certificates.CertFileName: []byte("es-cert-data"),
		},
	}
	entWithAssociation.SetAssociationConf(&commonv1.AssociationConf{CACertProvided: true, CASecretName: esTLSCertsSecret.Name})

	entWithoutTLS := *ent.DeepCopy()
	entWithoutTLS.Spec.HTTP.TLS = commonv1.TLSOptions{
		SelfSignedCertificate: &commonv1.SelfSignedCertificate{Disabled: true}}

	configSecret := corev1.Secret{
		Data: map[string][]byte{
			ConfigFilename:         []byte("config"),
			ReadinessProbeFilename: []byte("readiness-probe"),
		},
	}
	configSecret2 := corev1.Secret{
		Data: map[string][]byte{
			ConfigFilename:         []byte("another-config"),
			ReadinessProbeFilename: []byte("readiness-probe"),
		},
	}
	tlsCertsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: ent.Namespace, Name: certificates.InternalCertsSecretName(entv1.Namer, ent.Name)},
		Data: map[string][]byte{
			certificates.CertFileName: []byte("cert-data"),
		},
	}
	type args struct {
		c            k8s.Client
		ent          entv1.EnterpriseSearch
		configSecret corev1.Secret
	}
	tests := []struct {
		name     string
		args     args
		wantHash string
	}{
		{
			name: "happy path",
			args: args{
				c:            k8s.NewFakeClient(&configSecret, &esTLSCertsSecret, &tlsCertsSecret),
				ent:          entWithAssociation,
				configSecret: configSecret,
			},
			wantHash: "1696769747",
		},
		{
			name: "different config: different hash",
			args: args{
				c:            k8s.NewFakeClient(&configSecret2, &esTLSCertsSecret, &tlsCertsSecret),
				ent:          ent,
				configSecret: configSecret2,
			},
			wantHash: "3735097097",
		},
		{
			name: "no TLS configured: different hash",
			args: args{
				c:            k8s.NewFakeClient(&configSecret, &esTLSCertsSecret),
				ent:          entWithoutTLS,
				configSecret: configSecret,
			},
			wantHash: "275672346",
		},
		{
			name: "no ES association: different hash",
			args: args{
				c:            k8s.NewFakeClient(&configSecret, &tlsCertsSecret),
				ent:          ent,
				configSecret: configSecret,
			},
			wantHash: "1696769747",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := buildConfigHash(tt.args.c, tt.args.ent, tt.args.configSecret)
			require.NoError(t, err)
			require.Equal(t, tt.wantHash, hash)
		})
	}
}
