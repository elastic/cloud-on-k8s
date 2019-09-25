package detector

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDetect(t *testing.T) {
	testCases := []struct {
		name             string
		includeIndirect  bool
		wantDependencies *Dependencies
		wantErr          bool
	}{
		{
			name:            "All",
			includeIndirect: true,
			wantDependencies: &Dependencies{
				Indirect: mkIndirectDeps(),
				Direct:   mkDirectDeps(),
			},
		},
		{
			name:            "DirectOnly",
			includeIndirect: false,
			wantDependencies: &Dependencies{
				Direct: mkDirectDeps(),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.Open("testdata/deps.json")
			require.NoError(t, err)
			defer f.Close()

			gotDependencies, err := Detect(f, tc.includeIndirect)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.wantDependencies, gotDependencies)
		})
	}
}

func mkIndirectDeps() []LicenceInfo {
	return []LicenceInfo{
		{
			Module: Module{
				Path:     "github.com/davecgh/go-spew",
				Version:  "v1.1.0",
				Time:     mustParseTime("2016-10-29T20:57:26Z"),
				Indirect: true,
				Dir:      "testdata/github.com/davecgh/go-spew@v1.1.0",
			},
			LicenceFile: "testdata/github.com/davecgh/go-spew@v1.1.0/LICENCE.txt",
		},
		{
			Module: Module{
				Path:     "github.com/dgryski/go-minhash",
				Version:  "v0.0.0-20170608043002-7fe510aff544",
				Time:     mustParseTime("2017-06-08T04:30:02Z"),
				Indirect: true,
				Dir:      "testdata/github.com/dgryski/go-minhash@v0.0.0-20170608043002-7fe510aff544",
			},
			LicenceFile: "testdata/github.com/dgryski/go-minhash@v0.0.0-20170608043002-7fe510aff544/licence",
		},
		{
			Module: Module{
				Path:     "github.com/dgryski/go-spooky",
				Version:  "v0.0.0-20170606183049-ed3d087f40e2",
				Time:     mustParseTime("2017-06-06T18:30:49Z"),
				Indirect: true,
				Dir:      "testdata/github.com/dgryski/go-spooky@v0.0.0-20170606183049-ed3d087f40e2",
			},
			LicenceFile: "testdata/github.com/dgryski/go-spooky@v0.0.0-20170606183049-ed3d087f40e2/COPYING",
		},
	}
}

func mkDirectDeps() []LicenceInfo {
	return []LicenceInfo{
		{
			Module: Module{
				Path:    "github.com/ekzhu/minhash-lsh",
				Version: "v0.0.0-20171225071031-5c06ee8586a1",
				Time:    mustParseTime("2017-12-25T07:10:31Z"),
				Dir:     "testdata/github.com/ekzhu/minhash-lsh@v0.0.0-20171225071031-5c06ee8586a1",
			},
			Error: errLicenceNotFound,
		},
		{
			Module: Module{
				Path:    "gopkg.in/russross/blackfriday.v2",
				Version: "v2.0.1",
				Replace: &Module{
					Path:    "github.com/russross/blackfriday/v2",
					Version: "v2.0.1",
					Time:    mustParseTime("2018-09-20T17:16:15Z"),
					Dir:     "testdata/github.com/russross/blackfriday/v2@v2.0.1",
				},
				Dir: "testdata/github.com/russross/blackfriday/v2@v2.0.1",
			},
			LicenceFile: "testdata/github.com/russross/blackfriday/v2@v2.0.1/LICENSE.rst",
		},
	}
}

func mustParseTime(value string) *time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return &t
}
