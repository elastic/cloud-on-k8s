// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package run

import (
	"bufio"
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeWriteCloser fakes an io.WriteCloser. (https://github.com/golang/go/issues/22823)
type fakeWriteCloser struct {
	io.Writer
}

func (f *fakeWriteCloser) Close() error {
	return nil
}

func TestJSONLog(t *testing.T) {
	buf := new(bytes.Buffer)
	jl := newJSONLog(&fakeWriteCloser{Writer: buf})

	w := bufio.NewWriter(jl)
	_, err := w.WriteString(`
gibberish
{"key1": "value1", "key2": "value2"}
garbage
{"Time":"2019-10-30T10:52:39.933604025Z","Action":"output","Package":"github.com/elastic/cloud-on-k8s/test/e2e/es","Test":"TestVolumeEmptyDir/Creating_an_Elasticsearch_cluster_should_succeed","Output":"=== RUN   TestVolumeEmptyDir/Creating_an_Elasticsearch_cluster_should_succeed\n"}`)

	require.NoError(t, err)
	require.NoError(t, w.Flush())
	require.NoError(t, jl.Close())

	want := `{"key1": "value1", "key2": "value2"}
{"Time":"2019-10-30T10:52:39.933604025Z","Action":"output","Package":"github.com/elastic/cloud-on-k8s/test/e2e/es","Test":"TestVolumeEmptyDir/Creating_an_Elasticsearch_cluster_should_succeed","Output":"=== RUN   TestVolumeEmptyDir/Creating_an_Elasticsearch_cluster_should_succeed\n"}
`

	require.Equal(t, want, buf.String())
}
