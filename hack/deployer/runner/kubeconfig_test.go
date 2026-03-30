// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestMergeKubeconfigFiles(t *testing.T) {
	tests := []struct {
		name     string
		existing *clientcmdapi.Config
		new      *clientcmdapi.Config
		wantErr  bool
		verify   func(t *testing.T, result *clientcmdapi.Config)
	}{
		{
			name: "new cluster overrides stale entry",
			existing: &clientcmdapi.Config{
				Clusters: map[string]*clientcmdapi.Cluster{
					"kind-test": {Server: "https://127.0.0.1:12345"},
				},
				AuthInfos:      map[string]*clientcmdapi.AuthInfo{"kind-test": {}},
				Contexts:        map[string]*clientcmdapi.Context{"kind-test": {Cluster: "kind-test", AuthInfo: "kind-test"}},
				CurrentContext:  "kind-test",
			},
			new: &clientcmdapi.Config{
				Clusters: map[string]*clientcmdapi.Cluster{
					"kind-test": {Server: "https://127.0.0.1:54321"},
				},
				AuthInfos:      map[string]*clientcmdapi.AuthInfo{"kind-test": {}},
				Contexts:        map[string]*clientcmdapi.Context{"kind-test": {Cluster: "kind-test", AuthInfo: "kind-test"}},
				CurrentContext:  "kind-test",
			},
			verify: func(t *testing.T, result *clientcmdapi.Config) {
				t.Helper()
				if got := result.Clusters["kind-test"].Server; got != "https://127.0.0.1:54321" {
					t.Errorf("expected new server URL, got %s", got)
				}
			},
		},
		{
			name: "preserves unrelated clusters",
			existing: &clientcmdapi.Config{
				Clusters: map[string]*clientcmdapi.Cluster{
					"production": {Server: "https://prod.example.com"},
					"kind-old":   {Server: "https://127.0.0.1:11111"},
				},
				AuthInfos: map[string]*clientcmdapi.AuthInfo{
					"production": {},
					"kind-old":   {},
				},
				Contexts: map[string]*clientcmdapi.Context{
					"production": {Cluster: "production", AuthInfo: "production"},
					"kind-old":   {Cluster: "kind-old", AuthInfo: "kind-old"},
				},
				CurrentContext: "production",
			},
			new: &clientcmdapi.Config{
				Clusters:       map[string]*clientcmdapi.Cluster{"kind-new": {Server: "https://127.0.0.1:22222"}},
				AuthInfos:      map[string]*clientcmdapi.AuthInfo{"kind-new": {}},
				Contexts:       map[string]*clientcmdapi.Context{"kind-new": {Cluster: "kind-new", AuthInfo: "kind-new"}},
				CurrentContext: "kind-new",
			},
			verify: func(t *testing.T, result *clientcmdapi.Config) {
				t.Helper()
				if _, ok := result.Clusters["production"]; !ok {
					t.Error("production cluster was lost during merge")
				}
				if _, ok := result.Clusters["kind-old"]; !ok {
					t.Error("kind-old cluster was lost during merge")
				}
				if _, ok := result.Clusters["kind-new"]; !ok {
					t.Error("kind-new cluster was not added")
				}
				if result.CurrentContext != "kind-new" {
					t.Errorf("expected current-context kind-new, got %s", result.CurrentContext)
				}
			},
		},
		{
			name: "updates current context",
			existing: &clientcmdapi.Config{
				Clusters:       map[string]*clientcmdapi.Cluster{"old": {Server: "https://old.example.com"}},
				AuthInfos:      map[string]*clientcmdapi.AuthInfo{"old": {}},
				Contexts:       map[string]*clientcmdapi.Context{"old": {Cluster: "old", AuthInfo: "old"}},
				CurrentContext: "old",
			},
			new: &clientcmdapi.Config{
				Clusters:       map[string]*clientcmdapi.Cluster{"new": {Server: "https://new.example.com"}},
				AuthInfos:      map[string]*clientcmdapi.AuthInfo{"new": {}},
				Contexts:       map[string]*clientcmdapi.Context{"new": {Cluster: "new", AuthInfo: "new"}},
				CurrentContext: "new",
			},
			verify: func(t *testing.T, result *clientcmdapi.Config) {
				t.Helper()
				if result.CurrentContext != "new" {
					t.Errorf("expected current-context new, got %s", result.CurrentContext)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			existingPath := filepath.Join(dir, "existing")
			writeKubeconfig(t, existingPath, tt.existing)

			newPath := filepath.Join(dir, "new")
			writeKubeconfig(t, newPath, tt.new)

			err := mergeKubeconfigFiles(newPath, existingPath)
			if (err != nil) != tt.wantErr {
				t.Fatalf("mergeKubeconfigFiles() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			result, err := clientcmd.LoadFromFile(existingPath)
			if err != nil {
				t.Fatalf("failed to load merged kubeconfig: %v", err)
			}
			tt.verify(t, result)

			// Verify file permissions
			info, err := os.Stat(existingPath)
			if err != nil {
				t.Fatalf("failed to stat merged kubeconfig: %v", err)
			}
			if perm := info.Mode().Perm(); perm != 0600 {
				t.Errorf("expected file permissions 0600, got %04o", perm)
			}
		})
	}
}

func writeKubeconfig(t *testing.T, path string, cfg *clientcmdapi.Config) {
	t.Helper()
	data, err := clientcmd.Write(*cfg)
	if err != nil {
		t.Fatalf("failed to serialize kubeconfig: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("failed to write kubeconfig: %v", err)
	}
}
