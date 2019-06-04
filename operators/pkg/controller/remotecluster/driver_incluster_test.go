// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"reflect"
	"testing"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	esname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	ca1 = `-----BEGIN CERTIFICATE-----
MIIDKjCCAhKgAwIBAgIQV7oDCszRv6zQw50w1rhlBDANBgkqhkiG9w0BAQsFADAv
MRkwFwYDVQQLExA4OWo2ODhkOG1wNmh2NXRqMRIwEAYDVQQDEwl0cnVzdC1vbmUw
HhcNMTkwMzIxMjAzNzQ5WhcNMjAwMzIwMjAzODQ5WjAvMRkwFwYDVQQLExA4OWo2
ODhkOG1wNmh2NXRqMRIwEAYDVQQDEwl0cnVzdC1vbmUwggEiMA0GCSqGSIb3DQEB
AQUAA4IBDwAwggEKAoIBAQC9OSrzM7+1zjPkO2y8T2+9x+8wvKkO0YhGg610xkrV
bvWrzAt/RYIs6P1DjLEAWsT/L1ajZThxlkKAg+9/Xaji+9WD/bibaCukawVVakW9
fKoNCGdta83/s/QBL+lCIrNs7DH/gaGqxiw4gadwThkE/kyv4At6RAWDF1FedpYT
vxNqwblRGB3QJaBPMXofkZDhNHtMfYZE6DvyaJAQXhKMG+ytm2Z+SrXeDDwuQY8L
CUiagUnmac/NXfferSq4kKYylyr6Q3iIoVYSwvhdpKUjAJyiKuhnV/t5Gc8N+7yd
LN2AE4pY5D9XxtjIQ2XO4H0Eo31B1NqNr+SKSCzEieZVAgMBAAGjQjBAMA4GA1Ud
DwEB/wQEAwIChDAdBgNVHSUEFjAUBggrBgEFBQcDAQYIKwYBBQUHAwIwDwYDVR0T
AQH/BAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEAj0lokAuOgu+7LSSKvbFa5R6P
qzjkU1RW1Ajvab2VtEdAM65AwLUtZEXYEhWgajZArTu94MB0+/CPgPQSeSDIcJNM
Uf1NE9Mmn33s5QCTH+swPNyszNhjZIHoeCyt6/5nk0tnrNDvHS9agxSeGlNFGIwM
nnPMif73EGBpiC8EAzKWVOfXgTQwra0456NoIbIGnKaFAg8ZmzzEb3qxFBKY+uB6
2spp9BiNycCL662V8Uda2Gsnc29no5maHMzKcvEflPjyVuaKCWftimKKgcQiLD7z
zxQWYrr9DAi/E+MbXmKDSDfPf6N2uzV3XSGzPqSg4W6pSS73IV4whvGIrLGbIQ==
-----END CERTIFICATE-----`

	ca2 = `-----BEGIN CERTIFICATE-----
MIIDKzCCAhOgAwIBAgIRAK7i/u/wsh+i2G0yUygsJckwDQYJKoZIhvcNAQELBQAw
LzEZMBcGA1UECxMQNG1jZnhjbnh0ZjZuNHA5bDESMBAGA1UEAxMJdHJ1c3Qtb25l
MB4XDTE5MDMyMDIwNDg1NloXDTIwMDMxOTIwNDk1NlowLzEZMBcGA1UECxMQNG1j
Znhjbnh0ZjZuNHA5bDESMBAGA1UEAxMJdHJ1c3Qtb25lMIIBIjANBgkqhkiG9w0B
AQEFAAOCAQ8AMIIBCgKCAQEAu/Pws5FcyJw843pNow/Y95rApWAuGanU99DEmeOG
ggtpc3qtDWWKwLZ6cU+av3u82tf0HYSpy0Z2hn3PS2dGGgHPTr/tTGYA5alu1dn5
CgqQDBVLbkKA1lDcm8w98fRavRw6a0TX5DURqXs+smhdMztQjDNCl3kJ40JbXVAY
x5vhD2pKPCK0VIr9uYK0E/9dvrU0SJGLUlB+CY/DU7c8t22oer2T6fjCZzh3Fhwi
/aOKEwEUoE49orte0N9b1HSKlVePzIUuTTc3UU2ntWi96Uf2FesuAubU11WH4kIL
wRlofty7ewBzVmGte1fKUMjHB3mgb+WYwkEFwjpQL4LhkQIDAQABo0IwQDAOBgNV
HQ8BAf8EBAMCAoQwHQYDVR0lBBYwFAYIKwYBBQUHAwEGCCsGAQUFBwMCMA8GA1Ud
EwEB/wQFMAMBAf8wDQYJKoZIhvcNAQELBQADggEBAI+qczKQgkb5L5dXzn+KW92J
Sq1rrmaYUYLRTtPFH7t42REPYLs4UV0qR+6v/hJljQbAS+Vu3BioLWuxq85NsIjf
OK1KO7D8lwVI9tAetE0tKILqljTjwZpqfZLZ8fFqwzd9IM/WfoI7Z05k8BSL6XdM
FaRfSe/GJ+DR1dCwnWAVKGxAry4JSceVS9OXxYNRTcfQuT5s8h/6X5UaonTbhil7
91fQFaX8LSuZj23/3kgDTnjPmvj2sz5nODymI4YeTHLjdlMmTufWSJj901ITp7Bw
DMO3GhRADFpMz3vjHA2rHA4AQ6nC8N4lIYTw0AF1VAOC0SDntf6YEgrhRKRFAUY=
-----END CERTIFICATE-----`
)

var (
	sc = scheme.Scheme
)

func newFakeClient(t *testing.T, initialObjects []runtime.Object) k8s.Client {
	if err := v1alpha1.SchemeBuilder.AddToScheme(sc); err != nil {
		assert.Fail(t, "failed to build custom scheme")
	}
	return k8s.WrapClient(fake.NewFakeClient(initialObjects...))
}

func newCASecret(namespace, name, cert string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			certificates.CAFileName: []byte(cert),
		},
	}
}

func newRemoteInCluster(
	name, localNamespace, localName, remoteNamespace, remoteName string,
) *v1alpha1.RemoteCluster {
	return &v1alpha1.RemoteCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: localNamespace,
			Labels:    map[string]string{label.ClusterNameLabelName: localName},
		},
		Spec: v1alpha1.RemoteClusterSpec{
			Remote: v1alpha1.RemoteClusterRef{
				K8sLocalRef: commonv1alpha1.ObjectSelector{
					Name:      remoteName,
					Namespace: remoteNamespace,
				},
			},
		},
	}
}

func Test_apply(t *testing.T) {
	type args struct {
		rca           *ReconcileRemoteCluster
		remoteCluster v1alpha1.RemoteCluster
	}
	tests := []struct {
		name    string
		args    args
		want    v1alpha1.RemoteClusterStatus
		wantErr bool
	}{
		{
			name: "Phase successfully updated",
			args: args{
				rca: &ReconcileRemoteCluster{
					Client: newFakeClient(t, []runtime.Object{
						newCASecret("default", esname.CertsPublicSecretName("trust-one-es", certificates.TransportCAType), ca1),
						newCASecret("default", esname.CertsPublicSecretName("trust-two-es", certificates.TransportCAType), ca2),
						newRemoteInCluster(
							"remotecluster-sample-1-2",
							"default", "trust-one-es",
							"default", "trust-two-es",
						),
					}),
					watches:  watches.NewDynamicWatches(),
					recorder: &record.FakeRecorder{},
				},
				remoteCluster: *newRemoteInCluster(
					"remotecluster-sample-1-2",
					"default", "trust-one-es",
					"default", "trust-two-es",
				),
			},
			want: v1alpha1.RemoteClusterStatus{
				Phase:                  v1alpha1.RemoteClusterPropagated,
				ClusterName:            "trust-one-es",
				LocalTrustRelationship: "rc-remotecluster-sample-1-2",
				K8SLocalStatus: v1alpha1.LocalRefStatus{
					RemoteSelector: commonv1alpha1.ObjectSelector{
						Name:      "trust-two-es",
						Namespace: "default",
					},
					RemoteTrustRelationship: "rcr-remotecluster-sample-1-2-default",
				},
				SeedHosts: []string{"trust-two-es-es-remote-cluster-seed.default.svc:9300"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.args.rca.scheme == nil {
				tt.args.rca.scheme = sc
			}

			assert.NoError(t, tt.args.rca.watches.InjectScheme(tt.args.rca.scheme))

			got, err := doReconcile(tt.args.rca, tt.args.remoteCluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("apply() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("apply() = %v, want %v", got, tt.want)
			}
		})
	}
}
