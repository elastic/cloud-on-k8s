// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package shutdown

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/log"
)

var (
	singleShutdownFixture = `{
  "nodes": [
    {
      "node_id": "txXw-Kd2Q6K0PbYMAPzH-Q",
      "type": "REMOVE",
      "reason": "111800357",
      "shutdown_startedmillis": 1626780145861,
      "status": "COMPLETE",
      "shard_migration": {
        "status": "COMPLETE",
        "shard_migrations_remaining": 0
      },
      "persistent_tasks": {
        "status": "COMPLETE"
      },
      "plugins": {
        "status": "COMPLETE"
      }
    }
  ]
}
`
	shutdownFixture = `{
  "nodes": [
    {
      "node_id": "txXw-Kd2Q6K0PbYMAPzH-Q",
      "type": "REMOVE",
      "reason": "111800357",
      "shutdown_startedmillis": 1626780145861,
      "status": "COMPLETE",
      "shard_migration": {
        "status": "COMPLETE",
        "shard_migrations_remaining": 0
      },
      "persistent_tasks": {
        "status": "COMPLETE"
      },
      "plugins": {
        "status": "COMPLETE"
      }
    },
    {
      "node_id": "sh013PAoQFqkF92fBv1fzg",
      "type": "REMOVE",
      "reason": "111800357",
      "shutdown_startedmillis": 1626780145861,
      "status": "COMPLETE",
      "shard_migration": {
        "status": "COMPLETE",
        "shard_migrations_remaining": 0
      },
      "persistent_tasks": {
        "status": "COMPLETE"
      },
      "plugins": {
        "status": "COMPLETE"
      }
    }
  ]
}
`
	stalledShutdownFixture = `{
  "nodes": [
    {
      "node_id": "D_E3ZVdyQlOmc81NAUOD8w",
      "type": "REMOVE",
      "reason": "49124",
      "shutdown_startedmillis": 1637071630313,
      "status": "STALLED",
      "shard_migration": {
        "status": "STALLED",
        "shard_migrations_remaining": 4,
        "explanation": "shard [1] [primary] of index [elasticlogs_q-000001] cannot move, use the Cluster Allocation Explain API on this shard for details"
      },
      "persistent_tasks": {
        "status": "COMPLETE"
      },
      "plugins": {
        "status": "COMPLETE"
      }
    }
  ]
}
`
	noShutdownFixture = `{"nodes":[]}`
	ackFixture        = `{"acknowledged": true}`
)

func TestNodeShutdown_Clear(t *testing.T) {
	type args struct {
		typ    esclient.ShutdownType
		status *esclient.ShutdownStatus
	}
	tests := []struct {
		name       string
		args       args
		fixture    string
		wantErr    bool
		wantDelete bool
	}{
		{
			name:    "Respect type when deleting shutdowns",
			fixture: shutdownFixture,
			args: args{
				typ:    esclient.Restart,
				status: &esclient.ShutdownComplete,
			},
			wantErr:    false,
			wantDelete: false,
		},
		{
			name:    "Respect status when deleting shutdowns",
			fixture: shutdownFixture,
			args: args{
				typ:    esclient.Remove,
				status: &esclient.ShutdownInProgress,
			},
			wantErr:    false,
			wantDelete: false,
		},
		{
			name:    "Allow all status values when deleting shutdowns",
			fixture: shutdownFixture,
			args: args{
				typ:    esclient.Remove,
				status: nil,
			},
			wantErr:    false,
			wantDelete: true,
		},
		{
			name:    "Should delete shutdowns",
			fixture: shutdownFixture,
			args: args{
				typ:    esclient.Remove,
				status: &esclient.ShutdownComplete,
			},
			wantErr:    false,
			wantDelete: true,
		},
		{
			name: "Should bubble up errors",
			args: args{
				typ:    esclient.Remove,
				status: &esclient.ShutdownComplete,
			},
			fixture:    `{not json`,
			wantErr:    true,
			wantDelete: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deleteCalled := false
			client := esclient.NewMockClient(version.MustParse("7.15.2"), func(req *http.Request) *http.Response {
				if req.Method == http.MethodDelete {
					deleteCalled = true
				}
				return &http.Response{
					StatusCode: 200,
					Body:       ioutil.NopCloser(bytes.NewBuffer([]byte(tt.fixture))),
					Header:     make(http.Header),
					Request:    req,
				}
			})
			ns := &NodeShutdown{
				c:   client,
				typ: tt.args.typ,
				log: log.Log.WithName("test"),
			}

			if err := ns.Clear(context.Background(), tt.args.status); (err != nil) != tt.wantErr {
				t.Errorf("Clear() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantDelete != deleteCalled {
				t.Errorf("Clear() deletedCalled = %v, wantDelete %v", deleteCalled, tt.wantDelete)
			}
		})
	}
}

func TestNodeShutdown_ShutdownStatus(t *testing.T) {
	type args struct {
		podToNodeID map[string]string
		podName     string
	}
	tests := []struct {
		name    string
		args    args
		fixture string
		want    NodeShutdownStatus
		wantErr bool
	}{
		{
			name: "unknown node",
			args: args{
				podToNodeID: nil,
				podName:     "pod-1",
			},
			want:    NodeShutdownStatus{},
			wantErr: true,
		},
		{
			name:    "successful lookup",
			fixture: shutdownFixture,
			args: args{
				podToNodeID: map[string]string{
					"pod-1": "txXw-Kd2Q6K0PbYMAPzH-Q",
				},
				podName: "pod-1",
			},
			want: NodeShutdownStatus{
				Status:      esclient.ShutdownComplete,
				Explanation: "",
			},
			wantErr: false,
		},
		{
			name:    "successful lookup with explanation",
			fixture: stalledShutdownFixture,
			args: args{
				podToNodeID: map[string]string{
					"pod-1": "D_E3ZVdyQlOmc81NAUOD8w",
				},
				podName: "pod-1",
			},
			want: NodeShutdownStatus{
				Status:      esclient.ShutdownStalled,
				Explanation: "shard [1] [primary] of index [elasticlogs_q-000001] cannot move, use the Cluster Allocation Explain API on this shard for details",
			},
			wantErr: false,
		},
		{
			name:    "handles initialisation errors",
			args:    args{},
			fixture: "not json",
			want:    NodeShutdownStatus{},
			wantErr: true,
		},
		{
			name: "returns error when no shutdown in progress",
			args: args{
				podToNodeID: map[string]string{
					"pod-1": "txXw-Kd2Q6K0PbYMAPzH-Q",
				},
				podName: "pod-1",
			},
			fixture: noShutdownFixture,
			want:    NodeShutdownStatus{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := esclient.NewMockClient(version.MustParse("7.15.2"), func(req *http.Request) *http.Response {
				return &http.Response{
					StatusCode: 200,
					Body:       ioutil.NopCloser(bytes.NewBuffer([]byte(tt.fixture))),
					Header:     make(http.Header),
					Request:    req,
				}
			})
			ns := &NodeShutdown{
				c:           client,
				podToNodeID: tt.args.podToNodeID,
				log:         log.Log.WithName("test"),
			}
			got, err := ns.ShutdownStatus(context.Background(), tt.args.podName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ShutdownStatus() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ShutdownStatus() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNodeShutdown_ReconcileShutdowns(t *testing.T) {
	type args struct {
		typ          esclient.ShutdownType
		podToNodeID  map[string]string
		leavingNodes []string
	}
	tests := []struct {
		name        string
		args        args
		fixtures    []string
		wantErr     bool
		wantMethods []string
	}{
		{
			name: "no node leaving",
			args: args{
				typ:          esclient.Remove,
				podToNodeID:  nil,
				leavingNodes: nil,
			},
			fixtures: []string{
				noShutdownFixture,
			},
			wantErr:     false,
			wantMethods: []string{"GET"},
		},
		{
			name: "one node leaving",
			args: args{
				typ: esclient.Remove,
				podToNodeID: map[string]string{
					"pod-1": "txXw-Kd2Q6K0PbYMAPzH-Q",
				},
				leavingNodes: []string{"pod-1"},
			},
			fixtures: []string{
				noShutdownFixture,
				ackFixture,
				singleShutdownFixture,
			},
			wantErr:     false,
			wantMethods: []string{"GET", "PUT", "GET"},
		},
		{
			name: "two nodes leaving",
			args: args{
				typ: esclient.Remove,
				podToNodeID: map[string]string{
					"pod-1": "txXw-Kd2Q6K0PbYMAPzH-Q",
					"pod-2": "sh013PAoQFqkF92fBv1fzg",
				},
				leavingNodes: []string{"pod-1", "pod-2"},
			},
			fixtures: []string{
				noShutdownFixture,
				ackFixture,
				singleShutdownFixture,
				ackFixture,
				// technically incorrect as we are returning the same shutdown fixture as before. But we don't verify
				// the responses, so good enough for this test.
				singleShutdownFixture,
			},
			wantErr:     false,
			wantMethods: []string{"GET", "PUT", "GET", "PUT", "GET"},
		},
		{
			name: "shutdown already in progress",
			args: args{
				typ: esclient.Remove,
				podToNodeID: map[string]string{
					"pod-1": "txXw-Kd2Q6K0PbYMAPzH-Q",
				},
				leavingNodes: []string{"pod-1"},
			},
			fixtures: []string{
				shutdownFixture,
			},
			wantErr:     false,
			wantMethods: []string{"GET"},
		},
		{
			name: "unknown node",
			args: args{
				typ:          esclient.Remove,
				leavingNodes: []string{"pod-1"},
			},
			fixtures: []string{
				noShutdownFixture,
			},
			wantErr:     true,
			wantMethods: []string{"GET"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := 0
			var methodsCalled []string
			client := esclient.NewMockClient(version.MustParse("7.15.2"), func(req *http.Request) *http.Response {
				defer func() {
					i++
				}()
				methodsCalled = append(methodsCalled, req.Method)
				return &http.Response{
					StatusCode: 200,
					Body:       ioutil.NopCloser(bytes.NewBuffer([]byte(tt.fixtures[i]))),
					Header:     make(http.Header),
					Request:    req,
				}
			})
			ns := &NodeShutdown{
				c:           client,
				typ:         tt.args.typ,
				podToNodeID: tt.args.podToNodeID,
				log:         log.Log.WithName("test"),
			}
			if err := ns.ReconcileShutdowns(context.Background(), tt.args.leavingNodes); (err != nil) != tt.wantErr {
				t.Errorf("ReconcileShutdowns() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(methodsCalled, tt.wantMethods) {
				t.Errorf("ShutdownStatus() got = %v, want %v", methodsCalled, tt.wantMethods)
			}
		})
	}
}
