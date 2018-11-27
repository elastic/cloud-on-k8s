package daemon

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/protocol"
	"github.com/stretchr/testify/assert"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/drivers"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/drivers/empty"
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
	var mountReq = protocol.MountRequest{
		TargetDir: "pvc-id",
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
			s := NewTestServer(NewPersistentVolumeStub("pvc-id"))
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
			name: "Test Mount fails with empty response",
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
			name: "Test Mount Succeeds",
			args: args{
				driver: &empty.Driver{
					UnmountRes: flex.Success("successfully created the volume"),
				},
				req: httptest.NewRequest(http.MethodGet, "/unmount", bytes.NewReader(unmountReqBytes)),
			},
			want: flex.Success("successfully created the volume"),
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
