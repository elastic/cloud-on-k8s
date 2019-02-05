package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elastic/k8s-operators/local-volume/pkg/provider"

	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/drivers"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/drivers/empty"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/protocol"
	"github.com/elastic/k8s-operators/local-volume/pkg/k8s"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

func TestInitHandler(t *testing.T) {
	type args struct {
		driver drivers.Driver
		req    *http.Request
	}
	tests := []struct {
		name string
		args args
		want flex.Response
	}{
		{
			name: "Test Init",
			args: args{
				driver: &empty.Driver{},
				req:    httptest.NewRequest(http.MethodGet, "/init", nil),
			},
			want: flex.Response{
				Status:  flex.StatusSuccess,
				Message: "driver is available",
				Capabilities: flex.Capabilities{
					Attach: false, // only implement mount and unmount
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewTestServer()
			w := httptest.NewRecorder()
			handler := s.InitHandler()
			handler(w, tt.args.req)

			var body flex.Response
			json.NewDecoder(w.Result().Body).Decode(&body)
			assert.Equal(t, tt.want, body)
		})
	}
}

func TestMountHandler(t *testing.T) {
	pvcName := "pvc-name"
	var mountReq = protocol.MountRequest{
		TargetDir: "/path/" + pvcName,
	}
	mountReqBytes, _ := json.Marshal(mountReq)
	println(string(mountReqBytes))

	type args struct {
		driver drivers.Driver
		req    *http.Request
	}
	tests := []struct {
		name    string
		args    args
		want    flex.Response
		wantErr *http.Response
	}{
		{
			name: "Test Mount fails with empty response",
			args: args{
				driver: &empty.Driver{},
				req:    httptest.NewRequest(http.MethodGet, "/mount", nil),
			},
			wantErr: &http.Response{
				StatusCode: http.StatusInternalServerError,
				Status:     "500 Internal Server Error",
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header: http.Header{
					"X-Content-Type-Options": []string{"nosniff"},
					"Content-Type":           []string{"text/plain; charset=utf-8"},
				},
				ContentLength: -1,
				Body:          ioutil.NopCloser(bytes.NewReader([]byte("EOF\n"))),
			},
		},
		{
			name: "Test Mount Succeeds",
			args: args{
				driver: &empty.Driver{
					MountRes: flex.Success("successfully created the volume"),
				},
				req: httptest.NewRequest(http.MethodGet, "/mount", bytes.NewReader(mountReqBytes)),
			},
			want: flex.Success("successfully created the volume"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			s := NewTestServer(k8s.NewPersistentVolume(pvcName))
			s.driver = tt.args.driver
			handler := s.MountHandler()
			handler(w, tt.args.req)

			var body flex.Response
			var result = w.Result()
			if result.StatusCode > 300 {
				assert.Equal(t, tt.wantErr, result)
				return
			}
			json.NewDecoder(result.Body).Decode(&body)
			assert.Equal(t, tt.want, body)
			if tt.want.Status == flex.StatusSuccess {
				pv, err := s.k8sClient.ClientSet.CoreV1().PersistentVolumes().Get(pvcName, metav1.GetOptions{})
				assert.NoError(t, err)
				// make sure the PV node affinity was updated
				expectedAffinity := v1.NodeSelectorRequirement{
					Key:      apis.LabelHostname,
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{s.nodeName},
				}
				fmt.Println(pv.Spec)
				assert.Equal(t, pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0], expectedAffinity)
				// make sure the label was updated
				expectedLabel := s.nodeName
				actualLabel := pv.Labels[provider.NodeAffinityLabel]
				assert.Equal(t, expectedLabel, actualLabel)
			}
		})
	}
}

func TestUnmountHandler(t *testing.T) {
	var unmountReq = protocol.UnmountRequest{}
	unmountReqBytes, _ := json.Marshal(unmountReq)
	println(string(unmountReqBytes))

	type args struct {
		driver drivers.Driver
		req    *http.Request
	}
	tests := []struct {
		name    string
		args    args
		want    flex.Response
		wantErr *http.Response
	}{
		{
			name: "Test unmount fails with empty response",
			args: args{
				driver: &empty.Driver{},
				req:    httptest.NewRequest(http.MethodGet, "/unmount", nil),
			},
			wantErr: &http.Response{
				StatusCode: http.StatusInternalServerError,
				Status:     "500 Internal Server Error",
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header: http.Header{
					"X-Content-Type-Options": []string{"nosniff"},
					"Content-Type":           []string{"text/plain; charset=utf-8"},
				},
				ContentLength: -1,
				Body:          ioutil.NopCloser(bytes.NewReader([]byte("EOF\n"))),
			},
		},
		{
			name: "Test unmount Succeeds",
			args: args{
				driver: &empty.Driver{
					UnmountRes: flex.Success("successfully unmounted the volume"),
				},
				req: httptest.NewRequest(http.MethodGet, "/unmount", bytes.NewReader(unmountReqBytes)),
			},
			want: flex.Success("successfully unmounted the volume"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			s := NewTestServer()
			s.driver = tt.args.driver
			handler := s.UnmountHandler()
			handler(w, tt.args.req)

			var body flex.Response
			var result = w.Result()
			if result.StatusCode > 300 {
				assert.Equal(t, tt.wantErr, result)
				return
			}
			json.NewDecoder(result.Body).Decode(&body)
			assert.Equal(t, tt.want, body)
		})
	}
}
