// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validate

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elastic/cloud-on-k8s/hack/licence-detector/dependency"
	"github.com/stretchr/testify/require"
)

func TestValidateURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if r.Body != nil {
				_, _ = io.Copy(ioutil.Discard, r.Body)
				r.Body.Close()
			}
		}()

		if r.Method == http.MethodHead && r.URL.Query().Get("no_head") == "true" {
			http.Error(w, "method not supported", http.StatusMethodNotAllowed)
			return
		}

		if r.URL.Query().Get("valid") == "true" {
			fmt.Fprintln(w, "OK")
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	mkDepInfo := func(name string, valid, noHead bool) dependency.Info {
		return dependency.Info{
			Name: name,
			URL:  fmt.Sprintf("%s/%s?valid=%t&no_head=%t", server.URL, name, valid, noHead),
		}
	}

	testCases := []struct {
		name    string
		deps    *dependency.List
		wantErr bool
	}{
		{
			name: "AllValid",
			deps: &dependency.List{
				Direct:   []dependency.Info{mkDepInfo("a", true, false), mkDepInfo("b", true, false)},
				Indirect: []dependency.Info{mkDepInfo("c", true, false), mkDepInfo("d", true, false)},
			},
		},
		{
			name: "AllValidWithUnsupportedMethod",
			deps: &dependency.List{
				Direct:   []dependency.Info{mkDepInfo("a", true, false), mkDepInfo("b", true, true)},
				Indirect: []dependency.Info{mkDepInfo("c", true, false), mkDepInfo("d", true, true)},
			},
		},
		{
			name: "InvalidDirectDep",
			deps: &dependency.List{
				Direct:   []dependency.Info{mkDepInfo("a", true, false), mkDepInfo("b", false, false)},
				Indirect: []dependency.Info{mkDepInfo("c", true, false), mkDepInfo("d", true, false)},
			},
			wantErr: true,
		},
		{
			name: "InvalidDirectDepWithUnsupportedMethod",
			deps: &dependency.List{
				Direct:   []dependency.Info{mkDepInfo("a", true, false), mkDepInfo("b", false, true)},
				Indirect: []dependency.Info{mkDepInfo("c", true, false), mkDepInfo("d", true, false)},
			},
			wantErr: true,
		},
		{
			name: "InvalidIndirectDep",
			deps: &dependency.List{
				Direct:   []dependency.Info{mkDepInfo("a", true, false), mkDepInfo("b", true, false)},
				Indirect: []dependency.Info{mkDepInfo("c", true, false), mkDepInfo("d", false, false)},
			},
			wantErr: true,
		},
		{
			name: "InvalidIndirectDepWithUnsupportedMethod",
			deps: &dependency.List{
				Direct:   []dependency.Info{mkDepInfo("a", true, false), mkDepInfo("b", true, false)},
				Indirect: []dependency.Info{mkDepInfo("c", true, false), mkDepInfo("d", false, true)},
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateURLs(tc.deps)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
