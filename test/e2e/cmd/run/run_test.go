// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package run

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// FakeStreamProvider provides a new FakeStream
type FakeStreamProvider struct {
	withError, failed bool
	data              []byte
	stop              chan<- struct{}
}

// FakeStream can send an error in the middle of the stream
type FakeStream struct {
	*FakeStreamProvider
	pos    int
	reader io.Reader
}

func NewFakeStreamProvider(data []byte, stop chan<- struct{}, withError bool) StreamProvider {
	return &FakeStreamProvider{
		withError: withError,
		data:      data,
		stop:      stop,
	}
}

func (usp *FakeStreamProvider) NewStream() (io.ReadCloser, error) {
	return &FakeStream{
		FakeStreamProvider: usp,
		pos:                0,
		reader:             bytes.NewReader(usp.data),
	}, nil
}

// Read reads the underlying bytes or returns an error when half of the stream has been sent
func (us *FakeStream) Read(p []byte) (int, error) {
	n, err := us.reader.Read(p)
	us.pos = us.pos + n
	if us.withError && !us.failed && us.pos > len(us.data)/2 {
		us.failed = true
		return 0, fmt.Errorf("test error")
	}
	if err == io.EOF {
		// informs streamTestJobOutput that the end of the stream has been reached
		close(us.stop)
	}
	return n, err
}

func (us *FakeStream) Close() error {
	// noop
	return nil
}

// Test_helper_streamTestJobOutput_withError simulates an error while the stream is read using the FakeStreamProvider
func Test_helper_streamTestJobOutput_withError(t *testing.T) {

	log = logf.Log.WithName("streamTestJobOutput_withError")

	stopLogStream := make(chan struct{})
	sampleLogs, err := ioutil.ReadFile("testdata/stream.json")
	require.NoError(t, err)
	streamProvider := NewFakeStreamProvider(sampleLogs, stopLogStream, true)

	h := &helper{}
	streamErrors := make(chan error, 4096)
	writer := bytes.NewBuffer([]byte{})
	h.streamTestJobOutput(streamProvider, writer, streamErrors, stopLogStream, "test_pod")

	// Check that the data written are the
	got, err := ioutil.ReadAll(writer)
	require.NoError(t, err)

	// Check that we had one error
	errorCount := len(streamErrors)
	assert.Equal(t, 1, errorCount)

	assert.Equal(t, sampleLogs, got)
}

func Test_helper_streamTestJobOutput(t *testing.T) {

	log = logf.Log.WithName("streamTestJobOutput")

	stopLogStream := make(chan struct{})
	sampleLogs, err := ioutil.ReadFile("testdata/stream.json")
	require.NoError(t, err)
	streamProvider := NewFakeStreamProvider(sampleLogs, stopLogStream, false)

	h := &helper{}
	streamErrors := make(chan error, 4096)
	writer := bytes.NewBuffer([]byte{})
	h.streamTestJobOutput(streamProvider, writer, streamErrors, stopLogStream, "test_pod")

	// Check that the data written are the
	got, err := ioutil.ReadAll(writer)
	require.NoError(t, err)

	errorCount := len(streamErrors)
	assert.Equal(t, 0, errorCount)

	assert.Equal(t, sampleLogs, got)
}

func Test_parseLog(t *testing.T) {
	type args struct {
		line string
	}
	tests := []struct {
		name    string
		args    args
		want    time.Time
		wantErr bool
	}{
		{
			name: "happy path",
			args: args{
				line: `{"Time":"2020-08-19T07:55:30.02987855Z","Action":"output","Package":"github.com/elastic/cloud-on-k8s/test/e2e/beat","Test":"TestBeatKibanaRefWithTLSDisabled/All_expected_Pods_should_eventually_be_ready","Output":"=== RUN   TestBeatKibanaRefWithTLSDisabled/All_expected_Pods_should_eventually_be_ready\n"}`,
			},
			want: time.Date(2020, time.August, 19, 07, 55, 30, 29878550, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLog([]byte(tt.args.line))
			if (err != nil) != tt.wantErr {
				t.Errorf("parseLog() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseLog() got = %v, want %v", got, tt.want)
			}
		})
	}
}
