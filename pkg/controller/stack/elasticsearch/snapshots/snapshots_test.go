package snapshots

import (
	"testing"

	"github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestValidateSnapshotCredentials(t *testing.T) {
	tests := []struct {
		name string
		args map[string][]byte
		want error
	}{
		{
			name: "complete key file no error",
			args: map[string][]byte{
				"service-account.json": []byte(
					`{
                      "type": "service_account",
                      "project_id": "your-project-id",
                      "private_key_id": "...",
                      "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
                      "client_email": "service-account-for-your-repository@your-project-id.iam.gserviceaccount.com",
                      "client_id": "...",
                      "auth_uri": "https://accounts.google.com/o/oauth2/auth",
                      "token_uri": "https://accounts.google.com/o/oauth2/token",
                      "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
                      "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/your-bucket@your-project-id.iam.gserviceaccount.com"
                    }`),
			},
			want: nil,
		},
		{
			name: "missing keys cause an error",
			args: map[string][]byte{
				"something-missing": []byte(`{"type": "foo"}`),
			},
			want: errors.New("Expected keys [project_id private_key_id private_key client_email client_id auth_uri token_uri auth_provider_x509_cert_url client_x509_cert_url] not found in something-missing gcs credential file"),
		},
		{
			name: "non-Json data is an error",
			args: map[string][]byte{
				"something-missing": []byte("~?~"),
			},
			want: errors.New("gcs secrets need to be JSON, PKCS12 is not supported: invalid character '~' looking for beginning of value"),
		},
	}

	for _, tt := range tests {
		actual := ValidateSnapshotCredentials(v1alpha1.SnapshotRepositoryTypeGCS, tt.args)
		if tt.want != nil {
			assert.EqualError(t, actual, tt.want.Error())
		} else {
			assert.NoError(t, actual)
		}
	}
}
