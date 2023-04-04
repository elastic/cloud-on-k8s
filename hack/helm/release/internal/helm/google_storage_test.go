// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package helm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

const (
	validGeneration   int64 = 1680594142725727
	invalidGeneration int64 = 2340594149386234
	testBucketName          = "test-bucket"
)

var (
	// valid response data for both retrieving file attributes
	// and writing a new file to/from google storage JSON API.
	returnDataFmt = `{
		"kind": "storage#object",
		"id": "elastic-helm-charts-dev/index.yaml/%[1]d",
		"selfLink": "https://www.googleapis.com/storage/v1/b/elastic-helm-charts-dev/o/index.yaml",
		"mediaLink": "https://content-storage.googleapis.com/download/storage/v1/b/elastic-helm-charts-dev/o/index.yaml?generation=%[1]d&alt=media",
		"name": "index.yaml",
		"bucket": "elastic-helm-charts-dev",
		"generation": "%[1]d",
		"metageneration": "1",
		"contentType": "text/plain; charset=utf-8",
		"storageClass": "STANDARD",
		"size": "67750",
		"md5Hash": "0zxTfawe/vFq8rZX9Aa4bg==",
		"crc32c": "i1Pc8w==",
		"etag": "CN+Mjofdj/4CEAE=",
		"timeCreated": "2023-04-04T07:42:22.813Z",
		"updated": "2023-04-04T07:42:22.813Z",
		"timeStorageClassUpdated": "2023-04-04T07:42:22.813Z"
	  }`
	// valid error response format for google storage JSON API.
	errorResponseFmt = `{
		"error": {
		 "errors": [
		  {
		   "domain": "global",
		   "reason": "conditionNotMet",
		   "message": "At least one of the pre-conditions you specified did not hold.",
		   "locationType": "header",
		   "location": "If-Match"
		  }
		 ],
		 "code": %d,
		 "message": "At least one of the pre-conditions you specified did not hold."
		 }
		}`
)

func Test_readIndexFromBucket(t *testing.T) {
	tmpdir, err := os.MkdirTemp(os.TempDir(), "helm_google_storage_test")
	if err != nil {
		t.Fatalf("while creating temp dir: %s", err)
		return
	}
	defer func() {
		os.RemoveAll(tmpdir)
	}()
	type args struct {
		ctx    context.Context
		config readIndexConfig
	}
	tests := []struct {
		name    string
		args    args
		want    *index
		wantErr bool
	}{
		{
			name: "happy path",
			args: args{
				ctx: context.Background(),
				config: readIndexConfig{
					indexFile: filepath.Join(tmpdir, oldIndexFileName),
					bucket:    testBucketName,
				},
			},
			want: &index{
				generation: validGeneration,
				path:       filepath.Join(tmpdir, oldIndexFileName),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, ts := newTestStorageClient(t)
			if ts != nil {
				defer ts.Close()
			}
			tt.args.config.client = client
			got, err := readIndexFromBucket(tt.args.ctx, tt.args.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("readIndexFromBucket() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("readIndexFromBucket() = %v, want %v", got, tt.want)
			}
			if tt.want != nil {
				if _, err := os.Stat(tt.want.path); os.IsNotExist(err) {
					t.Errorf("readIndexFromBucket() = expecting file at %s but none was found", tt.want.path)
				}
			}
		})
	}
}

func newTestStorageClient(t *testing.T) (*storage.Client, *httptest.Server) {
	t.Helper()
	ts := testServer()
	client, err := storage.NewClient(
		context.Background(),
		option.WithEndpoint(ts.URL),
		option.WithCredentials(&google.Credentials{
			ProjectID: "fake",
			TokenSource: oauth2.StaticTokenSource(&oauth2.Token{
				AccessToken: "fake",
				Expiry:      time.Now().Add(1 * time.Hour),
			}),
		}))
	if err != nil {
		t.Fatalf("getting new storage client: %s", err)
		return nil, nil
	}
	return client, ts
}

func testServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read Attributes of index file (o.Attrs(ctx))
		if r.Method == http.MethodGet && strings.Contains(r.URL.String(), fmt.Sprintf("%s/o/index", testBucketName)) {
			fmt.Fprintln(w, fmt.Sprintf(returnDataFmt, validGeneration))
			return
		}
		// Read Index File data
		if r.Method == http.MethodGet && strings.Contains(r.URL.String(), fmt.Sprintf("%s/%s", testBucketName, indexFileName)) {
			fmt.Fprintln(w, `apiVersion: v1
  entries: []
  generated: ""`)
			return
		}
		// Write methods
		if r.Method == http.MethodPost && strings.Contains(r.URL.String(), fmt.Sprintf("/upload/storage/v1/b/%s/o", testBucketName)) {
			// If `ifGenerationMatch` query arg isn't present, just return valid response.
			if !strings.Contains(r.URL.String(), "ifGenerationMatch") {
				fmt.Fprintln(w, fmt.Sprintf(returnDataFmt, validGeneration))
				return
			}
			// If `ifGenerationMatch` query arg is present and matches the valid generation return valid response.
			if strings.Contains(r.URL.String(), fmt.Sprintf("ifGenerationMatch=%d", validGeneration)) {
				fmt.Fprintln(w, fmt.Sprintf(returnDataFmt, validGeneration))
				return
			}
			// If `ifGenerationMatch` query arg is present and matches the invalid generation return
			// precondition failed error response.
			if strings.Contains(r.URL.String(), fmt.Sprintf("ifGenerationMatch=%d", invalidGeneration)) {
				w.WriteHeader(http.StatusPreconditionFailed)
				fmt.Fprintln(w, fmt.Sprintf(errorResponseFmt, http.StatusPreconditionFailed))
				return
			}
		}
	}))
}

func Test_writeIndexToBucket(t *testing.T) {
	tmpdir, err := os.MkdirTemp(os.TempDir(), "helm_google_storage_test")
	if err != nil {
		t.Fatalf("while creating temp dir: %s", err)
		return
	}
	defer func() {
		os.RemoveAll(tmpdir)
	}()
	type args struct {
		ctx         context.Context
		config      writeIndexConfig
		fileHandler func() *os.File
	}
	tests := []struct {
		name    string
		args    args
		want    *index
		wantErr bool
	}{
		{
			name: "Write index with no existing index given writes a new index and a valid generation",
			args: args{
				ctx: context.Background(),
				config: writeIndexConfig{
					bucketFileName: indexFileName,
					bucket:         testBucketName,
					chartsRepoURL:  "http://fake.repo",
					existingIndex:  nil,
				},
				fileHandler: func() *os.File {
					f, err := os.Create(filepath.Join(tmpdir, indexFileName))
					if err != nil {
						t.Fatalf("while creating temp %s file: %s", indexFileName, err)
						return nil
					}
					return f
				},
			},
			want: &index{
				path:       filepath.Join(tmpdir, indexFileName),
				generation: validGeneration,
			},
			wantErr: false,
		},
		{
			name: "Write index given an existing index fails when the generation isn't what is expected",
			args: args{
				ctx: context.Background(),
				config: writeIndexConfig{
					bucketFileName: indexFileName,
					bucket:         testBucketName,
					chartsRepoURL:  "http://fake.repo",
					existingIndex: &index{
						path:       filepath.Join(tmpdir, indexFileName),
						generation: invalidGeneration,
					},
				},
				fileHandler: func() *os.File {
					f, err := os.Create(filepath.Join(tmpdir, indexFileName))
					if err != nil {
						t.Fatalf("while creating temp %s file: %s", indexFileName, err)
						return nil
					}
					return f
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Write index given an existing index succeeds when generation is what is expected",
			args: args{
				ctx: context.Background(),
				config: writeIndexConfig{
					bucketFileName: indexFileName,
					bucket:         testBucketName,
					chartsRepoURL:  "http://fake.repo",
					existingIndex: &index{
						path:       filepath.Join(tmpdir, indexFileName),
						generation: validGeneration,
					},
				},
				fileHandler: func() *os.File {
					f, err := os.Create(filepath.Join(tmpdir, indexFileName))
					if err != nil {
						t.Fatalf("while creating temp %s file: %s", indexFileName, err)
						return nil
					}
					return f
				},
			},
			want: &index{
				path:       filepath.Join(tmpdir, indexFileName),
				generation: 1680594142725727,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, ts := newTestStorageClient(t)
			if ts != nil {
				defer ts.Close()
			}
			tt.args.config.client = client
			var f *os.File
			if tt.args.fileHandler != nil {
				f = tt.args.fileHandler()
				tt.args.config.indexFileHandle = f
			}
			defer func() {
				if f != nil {
					f.Close()
				}
			}()
			got, err := writeIndexToBucket(tt.args.ctx, tt.args.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("writeIndexToBucket() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("writeIndexToBucket() = %v, want %v", got, tt.want)
			}
		})
	}
}
