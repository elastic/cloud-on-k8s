package client

import (
	"errors"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
)

func TestInit(t *testing.T) {
	type args struct {
		c Caller
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Succeeds",
			args: args{c: Caller{client: &http.Client{
				Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						Status:     "200 OK",
						StatusCode: http.StatusOK,
						Body:       ioutil.NopCloser(strings.NewReader(`{"message": "got initialised!"}`)),
					}, nil
				}),
			}}},
			want: `{"message": "got initialised!"}`,
		},
		{
			name: "Fails",
			args: args{c: Caller{client: &http.Client{
				Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
					return nil, errors.New("an error")
				}),
			}}},
			want: `Get http://unix/init: an error`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Init(tt.args.c); got != tt.want {
				t.Errorf("Init() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMount(t *testing.T) {
	type args struct {
		c    Caller
		args []string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Succeeds",
			args: args{
				args: []string{"/apath"},
				c: Caller{
					client: &http.Client{
						Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								Status:     "200 OK",
								StatusCode: http.StatusOK,
								Body:       ioutil.NopCloser(strings.NewReader(`{"message": "got mounted!"}`)),
							}, nil
						}),
					},
				},
			},
			want: `{"message": "got mounted!"}`,
		},
		{
			name: "Succeeds in anotherpath",
			args: args{
				args: []string{"/anotherpath"},
				c: Caller{
					client: &http.Client{
						Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								Status:     "200 OK",
								StatusCode: http.StatusOK,
								Body:       ioutil.NopCloser(strings.NewReader(`{"message": "got mounted in anotherpath!"}`)),
							}, nil
						}),
					},
				},
			},
			want: `{"message": "got mounted in anotherpath!"}`,
		},
		{
			name: "Succeeds with extra settings",
			args: args{
				args: []string{"/anotherpath", `{"sizeBytes": "2048"}`},
				c: Caller{
					client: &http.Client{
						Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								Status:     "200 OK",
								StatusCode: http.StatusOK,
								Body:       ioutil.NopCloser(strings.NewReader(`{"message": "got mounted in anotherpath with extras settings!"}`)),
							}, nil
						}),
					},
				},
			},
			want: `{"message": "got mounted in anotherpath with extras settings!"}`,
		},
		{
			name: "Fails",
			args: args{
				args: []string{"/apath"},
				c: Caller{
					client: &http.Client{
						Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
							return nil, errors.New("some error happened")
						}),
					},
				},
			},
			want: `Post http://unix/mount: some error happened`,
		},
		{
			name: "Fails due to 2nd parameter not being json",
			args: args{
				args: []string{"/apath", "uhah"},
				c: Caller{
					client: &http.Client{
						Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
							return nil, errors.New("some error happened")
						}),
					},
				},
			},
			want: `invalid character 'u' looking for beginning of value`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Mount(tt.args.c, tt.args.args); got != tt.want {
				t.Errorf("Mount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnmount(t *testing.T) {
	type args struct {
		c    Caller
		args []string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Succeeds",
			args: args{
				args: []string{"/apath"},
				c: Caller{
					client: &http.Client{
						Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								Status:     "200 OK",
								StatusCode: http.StatusOK,
								Body:       ioutil.NopCloser(strings.NewReader(`{"message": "got unmounted!"}`)),
							}, nil
						}),
					},
				},
			},
			want: `{"message": "got unmounted!"}`,
		},
		{
			name: "Fails",
			args: args{
				args: []string{"/apath"},
				c: Caller{
					client: &http.Client{
						Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
							return nil, errors.New("some error happened while unmounting")
						}),
					},
				},
			},
			want: `Post http://unix/unmount: some error happened while unmounting`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Unmount(tt.args.c, tt.args.args); got != tt.want {
				t.Errorf("Unmount() = %v, want %v", got, tt.want)
			}
		})
	}
}
