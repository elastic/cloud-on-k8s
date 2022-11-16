// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package run

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// FakeLogStreamProvider provides a new FakeLogStream
type FakeLogStreamProvider struct {
	withError, failed bool
	data              []byte
	stop              chan<- struct{}
}

// FakeLogStream can send an error in the middle of the stream
type FakeLogStream struct {
	*FakeLogStreamProvider
	pos    int
	reader io.Reader
}

// NewFakeLogStreamProvider returns a new fake LogStreamProvider. If withError is true the FakeLogStream
// will return an one-time error when reaching half of the stream.
func NewFakeLogStreamProvider(data []byte, stop chan<- struct{}, withError bool) LogStreamProvider {
	return &FakeLogStreamProvider{
		withError: withError,
		data:      data,
		stop:      stop,
	}
}

func (f *FakeLogStreamProvider) NewLogStream() (io.ReadCloser, error) {
	return &FakeLogStream{
		FakeLogStreamProvider: f,
		pos:                   0,
		reader:                bytes.NewReader(f.data),
	}, nil
}

func (f *FakeLogStreamProvider) String() string {
	return "test_pod"
}

// Read reads the underlying bytes or returns an error when half of the stream has been sent
func (us *FakeLogStream) Read(p []byte) (int, error) {
	n, err := us.reader.Read(p)
	us.pos += n
	if us.withError && !us.failed && us.pos > len(us.data)/2 {
		us.failed = true
		return 0, fmt.Errorf("test error")
	}
	if errors.Is(err, io.EOF) {
		// informs streamTestJobOutput that the end of the stream has been reached
		close(us.stop)
	}
	return n, err
}

func (us *FakeLogStream) Close() error {
	// noop
	return nil
}

// Test_helper_streamTestJobOutput_withError simulates an error while the stream is read using the FakeLogStreamProvider
func Test_helper_streamTestJobOutput_withError(t *testing.T) {
	log = logf.Log.WithName("streamTestJobOutput_withError")

	stopLogStream := make(chan struct{})
	sampleLogs, err := os.ReadFile("testdata/stream.json")
	require.NoError(t, err)
	streamProvider := NewFakeLogStreamProvider(sampleLogs, stopLogStream, true)

	streamErrors := make(chan error, 4096)
	writer := bytes.NewBuffer([]byte{})
	streamTestJobOutput(streamProvider, goLangTestTimestampParser, writer, streamErrors, stopLogStream)

	got, err := io.ReadAll(writer)
	require.NoError(t, err)

	// Check that we had one error
	errorCount := len(streamErrors)
	assert.Equal(t, 1, errorCount)

	// Check that the data are the expected ones
	assert.Equal(t, sampleLogs, got)
}

func Test_helper_streamTestJobOutput(t *testing.T) {
	log = logf.Log.WithName("streamTestJobOutput")

	stopLogStream := make(chan struct{})
	sampleLogs, err := os.ReadFile("testdata/stream.json")
	require.NoError(t, err)
	streamProvider := NewFakeLogStreamProvider(sampleLogs, stopLogStream, false)

	streamErrors := make(chan error, 4096)
	writer := bytes.NewBuffer([]byte{})
	streamTestJobOutput(streamProvider, goLangTestTimestampParser, writer, streamErrors, stopLogStream)

	// Check that the data are the expected ones
	got, err := io.ReadAll(writer)
	require.NoError(t, err)

	errorCount := len(streamErrors)
	assert.Equal(t, 0, errorCount)

	assert.Equal(t, sampleLogs, got)
}

func Test_goLangTestTimestampParser(t *testing.T) {
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
				line: `{"Time":"2020-08-19T07:55:30.02987855Z","Action":"output","Package":"github.com/elastic/cloud-on-k8s/v2/test/e2e/beat","Test":"TestBeatKibanaRefWithTLSDisabled/All_expected_Pods_should_eventually_be_ready","Output":"=== RUN   TestBeatKibanaRefWithTLSDisabled/All_expected_Pods_should_eventually_be_ready\n"}`,
			},
			want: time.Date(2020, time.August, 19, 07, 55, 30, 29878550, time.UTC),
		},
		{
			name: "corrupted timestamp",
			args: args{
				line: `{"Time":"2020-08-19T07:55:30.02987855Z2020-08-19T07:55:30.02987855Z","Output":"=== RUN   TestBeatKibanaRefWithTLSDisabled/All_expected_Pods_should_eventually_be_ready\n"}`,
			},
			wantErr: true,
		},
		{
			name: "corrupted json",
			args: args{
				line: `{"Time":"2020-08-19T07:55:30.02987855Z,"Output":"=== RUN   TestBeatKibanaRefWithTLSDisabled/All_expected_Pods_should_eventually_be_ready\n"}`,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := goLangTestTimestampParser([]byte(tt.args.line))
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

func Test_stdTimestampParser(t *testing.T) {
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
				line: `{"log.level":"info","@timestamp":"2020-09-02T13:38:38.064Z","log.logger":"chaos","message":"Leader collection","service.version":"0.0.0-SNAPSHOT+00000000","service.type":"eck","ecs.version":"1.4.0"}`,
			},
			want: time.Date(2020, time.September, 02, 13, 38, 38, 64000000, time.UTC),
		},
		{
			name: "corrupted timestamp",
			args: args{
				line: `{"log.level":"info","@timestamp":"2020-09-02T1E3:38:38.064Z","log.logger":"chaos","message":"Leader collection","service.version":"0.0.0-SNAPSHOT+00000000","service.type":"eck","ecs.version":"1.4.0"}`,
			},
			wantErr: true,
		},
		{
			name: "corrupted json",
			args: args{
				line: `{"log.level":"info","@timestamp":2020-09-02T13:38:38.064Z","log.logger":"chaos","message":"Leader collection","service.version":"0.0.0-SNAPSHOT+00000000","service.type":"eck","ecs.version":"1.4.0"}`,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := stdTimestampParser([]byte(tt.args.line))
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
