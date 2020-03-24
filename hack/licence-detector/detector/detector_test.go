// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package detector

import (
	"os"
	"testing"

	"github.com/elastic/cloud-on-k8s/hack/licence-detector/dependency"
	"github.com/stretchr/testify/require"
)

func TestDetect(t *testing.T) {
	testCases := []struct {
		name             string
		includeIndirect  bool
		overrides        dependency.Overrides
		wantDependencies func() *dependency.List
		wantErr          bool
	}{
		{
			name:            "All",
			includeIndirect: true,
			wantDependencies: func() *dependency.List {
				return &dependency.List{
					Indirect: mkIndirectDeps(),
					Direct:   mkDirectDeps(),
				}
			},
		},
		{
			name:            "DirectOnly",
			includeIndirect: false,
			wantDependencies: func() *dependency.List {
				return &dependency.List{
					Direct: mkDirectDeps(),
				}
			},
		},
		{
			name:            "WithOverrides",
			includeIndirect: true,
			overrides: map[string]dependency.Info{
				"github.com/davecgh/go-spew":         dependency.Info{Name: "github.com/davecgh/go-spew", URL: "http://example.com/go-spew"},
				"github.com/russross/blackfriday/v2": dependency.Info{Name: "github.com/russross/blackfriday/v2", LicenceType: "MIT"},
			},
			wantDependencies: func() *dependency.List {
				deps := &dependency.List{}

				for _, d := range mkIndirectDeps() {
					d := d
					if d.Name == "github.com/davecgh/go-spew" {
						d.URL = "http://example.com/go-spew"
					}
					deps.Indirect = append(deps.Indirect, d)
				}

				for _, d := range mkDirectDeps() {
					d := d
					if d.Name == "github.com/russross/blackfriday/v2" {
						d.LicenceType = "MIT"
					}
					deps.Direct = append(deps.Direct, d)
				}

				return deps
			},
		},
		{
			name:            "WithInvalidLicenceFileOverride",
			includeIndirect: true,
			overrides: map[string]dependency.Info{
				"github.com/davecgh/go-spew":         dependency.Info{Name: "github.com/davecgh/go-spew", LicenceFile: "/path/to/nowhere"},
				"github.com/russross/blackfriday/v2": dependency.Info{Name: "github.com/russross/blackfriday/v2", LicenceFile: "/path/to/nowhere"},
			},
			wantErr: true,
		},

		{
			name:            "LicenceNotWhitelisted",
			includeIndirect: true,
			overrides: map[string]dependency.Info{
				"github.com/davecgh/go-spew":         dependency.Info{Name: "github.com/davecgh/go-spew", LicenceType: "Totally Legit License 2.0"},
				"github.com/russross/blackfriday/v2": dependency.Info{Name: "github.com/russross/blackfriday/v2", LicenceType: "MIT"},
				"github.com/davecgh/go-gk":           dependency.Info{Name: "github.com/davecgh/go-spew", LicenceType: "UNKNOWN"},
			},
			wantErr: true,
		},
	}

	// create classifier
	classifier, err := NewClassifier("testdata/licence.db")
	require.NoError(t, err)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.Open("testdata/deps.json")
			require.NoError(t, err)
			defer f.Close()

			gotDependencies, err := Detect(f, classifier, tc.overrides, tc.includeIndirect)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.wantDependencies(), gotDependencies)
		})
	}
}

func mkIndirectDeps() []dependency.Info {
	return []dependency.Info{
		{
			Name:        "github.com/davecgh/go-spew",
			Version:     "v1.1.0",
			VersionTime: "2016-10-29T20:57:26Z",
			Dir:         "testdata/github.com/davecgh/go-spew@v1.1.0",
			LicenceType: "ISC",
			LicenceFile: "testdata/github.com/davecgh/go-spew@v1.1.0/LICENCE.txt",
			URL:         "https://github.com/davecgh/go-spew",
		},
		{
			Name:        "github.com/dgryski/go-minhash",
			Version:     "v0.0.0-20170608043002-7fe510aff544",
			VersionTime: "2017-06-08T04:30:02Z",
			Dir:         "testdata/github.com/dgryski/go-minhash@v0.0.0-20170608043002-7fe510aff544",
			LicenceType: "MIT",
			LicenceFile: "testdata/github.com/dgryski/go-minhash@v0.0.0-20170608043002-7fe510aff544/licence",
			URL:         "https://github.com/dgryski/go-minhash",
		},
		{
			Name:        "github.com/dgryski/go-spooky",
			Version:     "v0.0.0-20170606183049-ed3d087f40e2",
			VersionTime: "2017-06-06T18:30:49Z",
			Dir:         "testdata/github.com/dgryski/go-spooky@v0.0.0-20170606183049-ed3d087f40e2",
			LicenceType: "MIT",
			LicenceFile: "testdata/github.com/dgryski/go-spooky@v0.0.0-20170606183049-ed3d087f40e2/COPYING",
			URL:         "https://github.com/dgryski/go-spooky",
		},
	}
}

func mkDirectDeps() []dependency.Info {
	return []dependency.Info{
		{
			Name:        "github.com/ekzhu/minhash-lsh",
			Version:     "v0.0.0-20171225071031-5c06ee8586a1",
			VersionTime: "2017-12-25T07:10:31Z",
			Dir:         "testdata/github.com/ekzhu/minhash-lsh@v0.0.0-20171225071031-5c06ee8586a1",
			LicenceType: "MIT",
			LicenceFile: "testdata/github.com/ekzhu/minhash-lsh@v0.0.0-20171225071031-5c06ee8586a1/licence.txt",
			URL:         "https://github.com/ekzhu/minhash-lsh",
		},
		{
			Name:        "github.com/russross/blackfriday/v2",
			Version:     "v2.0.1",
			VersionTime: "2018-09-20T17:16:15Z",
			Dir:         "testdata/github.com/russross/blackfriday/v2@v2.0.1",
			LicenceType: "BSD-2-Clause",
			LicenceFile: "testdata/github.com/russross/blackfriday/v2@v2.0.1/LICENSE.rst",
			URL:         "https://github.com/russross/blackfriday",
		},
	}
}

func TestDetermineURL(t *testing.T) {
	testCases := []struct {
		name     string
		override string
		modPath  string
		want     string
	}{
		{
			name:     "WithOverride",
			override: "https://go.elast.co/dep",
			modPath:  "github.com/elastic/dep/path",
			want:     "https://go.elast.co/dep",
		},
		{
			name:    "WithNonGitHubPath",
			modPath: "go.uber.org/zap",
			want:    "https://go.uber.org/zap",
		},
		{
			name:    "WithValidGitHubPath",
			modPath: "github.com/elastic/cloud-on-k8s",
			want:    "https://github.com/elastic/cloud-on-k8s",
		},
		{
			name:    "WithInvalidGitHubPath",
			modPath: "github.com/elastic/cloud-on-k8s/api/v1/elasticsearch",
			want:    "https://github.com/elastic/cloud-on-k8s",
		},
		{
			name:    "WithK8sPath",
			modPath: "k8s.io/apimachinery",
			want:    "https://github.com/kubernetes/apimachinery",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			have := determineURL(tc.override, tc.modPath)
			require.Equal(t, tc.want, have)
		})
	}
}
