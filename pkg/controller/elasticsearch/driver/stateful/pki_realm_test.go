// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver/shared"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/rolemapping"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// fakePKIRealmClient returns a node for each entry of nodeAttributes, keyed by node name, with the value of the
// ECK-managed PKI realm node attribute, or without it when the value is empty. The calls fail with the errors of errs,
// in order, then return the nodes of earlierNodeAttributes, in order, then the nodes of nodeAttributes.
// The cluster state requests fail with the errors of clusterStateErrs, in order, then report the file-based settings
// returned by fileSettings for the version of the file-settings Secret of es, or that version when fileSettings is
// nil.
type fakePKIRealmClient struct {
	esclient.Client
	nodeAttributes        map[string]string
	earlierNodeAttributes []map[string]string
	errs                  []error
	queried               bool

	k8sClient           k8s.Client
	es                  types.NamespacedName
	fileSettings        func(persisted int64) esclient.FileSettings
	clusterStateErrs    []error
	clusterStateQueried bool
}

func (f *fakePKIRealmClient) GetClusterState(ctx context.Context) (esclient.ClusterState, error) {
	f.clusterStateQueried = true
	if len(f.clusterStateErrs) > 0 {
		err := f.clusterStateErrs[0]
		f.clusterStateErrs = f.clusterStateErrs[1:]
		return esclient.ClusterState{}, err
	}
	var secret corev1.Secret
	if err := f.k8sClient.Get(ctx, types.NamespacedName{Namespace: f.es.Namespace, Name: esv1.FileSettingsSecretName(f.es.Name)}, &secret); err != nil {
		return esclient.ClusterState{}, err
	}
	var settings filesettings.Settings
	if err := json.Unmarshal(secret.Data[filesettings.SettingsSecretKey], &settings); err != nil {
		return esclient.ClusterState{}, err
	}
	persisted, err := strconv.ParseInt(settings.Metadata.Version, 10, 64)
	if err != nil {
		return esclient.ClusterState{}, err
	}
	fileSettings := esclient.FileSettings{Version: persisted}
	if f.fileSettings != nil {
		fileSettings = f.fileSettings(persisted)
	}
	var clusterState esclient.ClusterState
	clusterState.Metadata.ReservedState.FileSettings = fileSettings
	return clusterState, nil
}

func (f *fakePKIRealmClient) GetNodes(_ context.Context) (esclient.Nodes, error) {
	f.queried = true
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		return esclient.Nodes{}, err
	}
	nodeAttributes := f.nodeAttributes
	if len(f.earlierNodeAttributes) > 0 {
		nodeAttributes = f.earlierNodeAttributes[0]
		f.earlierNodeAttributes = f.earlierNodeAttributes[1:]
	}
	nodes := esclient.Nodes{Nodes: make(map[string]esclient.Node, len(nodeAttributes))}
	for name, val := range nodeAttributes {
		node := esclient.Node{Name: name, Attributes: map[string]string{"zone": "a"}}
		if val != "" {
			node.Attributes[commonv1.ECKPKIRealmNodeAttribute] = val
		}
		nodes.Nodes["id-"+name] = node
	}
	return nodes, nil
}

func pkiRealmTestPod(name string, ready bool) corev1.Pod {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	return corev1.Pod{
		Name: name,
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{
			{Type: corev1.PodReady, Status: status},
			{Type: corev1.ContainersReady, Status: status},
		}},
	}
}

func TestPKIRealmRunningOnAllNodes(t *testing.T) {
	tests := []struct {
		name           string
		esReachable    bool
		pods           []corev1.Pod
		nodeAttributes map[string]string
		errs           []error
		want           bool
		wantErr        bool
	}{
		{
			name:           "all nodes run the realm",
			esReachable:    true,
			pods:           []corev1.Pod{pkiRealmTestPod("es-0", true), pkiRealmTestPod("es-1", true)},
			nodeAttributes: map[string]string{"es-0": "true", "es-1": "true"},
			want:           true,
		},
		{
			name:           "a node of the cluster did not load the realm yet",
			esReachable:    true,
			pods:           []corev1.Pod{pkiRealmTestPod("es-0", true), pkiRealmTestPod("es-1", true)},
			nodeAttributes: map[string]string{"es-0": "true", "es-1": ""},
			want:           false,
		},
		{
			name:           "a node reports the attribute with another value",
			esReachable:    true,
			pods:           []corev1.Pod{pkiRealmTestPod("es-0", true), pkiRealmTestPod("es-1", true)},
			nodeAttributes: map[string]string{"es-0": "true", "es-1": "false"},
			want:           false,
		},
		{
			name:           "a Pod that is not Ready is missing from the nodes of the cluster: it may rejoin with a previous configuration",
			esReachable:    true,
			pods:           []corev1.Pod{pkiRealmTestPod("es-0", true), pkiRealmTestPod("es-1", true), pkiRealmTestPod("es-2", false)},
			nodeAttributes: map[string]string{"es-0": "true", "es-1": "true"},
			want:           false,
			wantErr:        true,
		},
		{
			name:           "a Pod that is not Ready but a node of the cluster with the realm",
			esReachable:    true,
			pods:           []corev1.Pod{pkiRealmTestPod("es-0", true), pkiRealmTestPod("es-1", false)},
			nodeAttributes: map[string]string{"es-0": "true", "es-1": "true"},
			want:           true,
		},
		{
			name:           "a Pod that is not Ready but still a node of the cluster without the realm",
			esReachable:    true,
			pods:           []corev1.Pod{pkiRealmTestPod("es-0", true), pkiRealmTestPod("es-1", false)},
			nodeAttributes: map[string]string{"es-0": "true", "es-1": ""},
			want:           false,
		},
		{
			name:           "a Ready Pod is missing from the nodes of the cluster",
			esReachable:    true,
			pods:           []corev1.Pod{pkiRealmTestPod("es-0", true), pkiRealmTestPod("es-1", true)},
			nodeAttributes: map[string]string{"es-0": "true"},
			want:           false,
			wantErr:        true,
		},
		{
			name:           "a Pod is missing from the nodes of the cluster and a node did not load the realm yet",
			esReachable:    true,
			pods:           []corev1.Pod{pkiRealmTestPod("es-0", true), pkiRealmTestPod("es-1", true), pkiRealmTestPod("es-2", true)},
			nodeAttributes: map[string]string{"es-0": "true", "es-1": ""},
			want:           false,
		},
		{
			name:           "no node in the response",
			esReachable:    true,
			pods:           []corev1.Pod{pkiRealmTestPod("es-0", false)},
			nodeAttributes: map[string]string{},
			want:           false,
			wantErr:        true,
		},
		{
			name:           "Elasticsearch is not reachable",
			esReachable:    false,
			pods:           []corev1.Pod{pkiRealmTestPod("es-0", true)},
			nodeAttributes: map[string]string{"es-0": "true"},
			want:           false,
			wantErr:        true,
		},
		{
			name:           "the nodes request fails",
			esReachable:    true,
			pods:           []corev1.Pod{pkiRealmTestPod("es-0", true)},
			nodeAttributes: map[string]string{"es-0": "true"},
			errs:           []error{errors.New("boom")},
			want:           false,
			wantErr:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pkiRealmRunningOnAllNodes(t.Context(), tt.esReachable,
				&fakePKIRealmClient{nodeAttributes: tt.nodeAttributes, errs: tt.errs}, &reconcile.ResourcesState{AllPods: tt.pods})
			require.Equal(t, tt.wantErr, err != nil)
			require.Equal(t, tt.want, got)
		})
	}
}

// pkiRealmTestRequest returns a PKI role mapping request Secret of es, with the given evaluation annotation if not
// empty.
func pkiRealmTestRequest(es esv1.Elasticsearch, evaluation string) *corev1.Secret {
	secret := &corev1.Secret{
		Namespace: es.Namespace, Name: "kb-client-cert", Labels: rolemapping.RequestLabels(es),
		Data: map[string][]byte{
			rolemapping.RolesField:                []byte("kibana_system"),
			rolemapping.PublicKeyFingerprintField: []byte("fp"),
			rolemapping.ResourceUIDField:          []byte("uid-kb"),
			rolemapping.AssociationNameField:      []byte("kb-es"),
		},
	}
	if evaluation != "" {
		secret.Annotations = map[string]string{rolemapping.PKIEvaluationAnnotation: evaluation}
	}
	return secret
}

func TestReconcilePKIRoleMappings(t *testing.T) {
	es := esv1.Elasticsearch{Namespace: "ns", Name: "es"}
	pods := []corev1.Pod{pkiRealmTestPod("es-0", true), pkiRealmTestPod("es-1", true)}
	evaluated, err := json.Marshal(rolemapping.Evaluation{PublicKeyFingerprint: "fp", Roles: "kibana_system"})
	require.NoError(t, err)
	tests := []struct {
		name               string
		pkiRealmConfigured bool
		paused             bool
		esReachable        bool
		nodeAttributes     map[string]string
		// getNodesErrs are the errors of the first nodes requests, which then succeed.
		getNodesErrs []error
		// earlierNodeAttributes are the node attributes of the first successful nodes requests, before nodeAttributes.
		earlierNodeAttributes []map[string]string
		// request is the evaluation annotation of the request Secret: nil means no request Secret.
		request *string
		// conflicting adds a request with the same public key fingerprint asking for other roles.
		conflicting bool
		// fileSettings are the file-based settings Elasticsearch reports for the persisted version, which it reports
		// as applied when nil.
		fileSettings func(persisted int64) esclient.FileSettings
		// clusterStateErrs are the errors of the first cluster state requests, which then succeed.
		clusterStateErrs        []error
		wantESQueried           bool
		wantClusterStateQueried bool
		// wantRequeues tells if each successive reconciliation requeues. The other expectations apply after the last.
		wantRequeues   []bool
		wantEvaluation string
		// wantConflict expects the request to be evaluated with a conflict, instead of wantEvaluation.
		wantConflict bool
	}{
		{
			name:               "realm not configured: annotation removed without querying Elasticsearch",
			pkiRealmConfigured: false,
			esReachable:        true,
			request:            new(string(evaluated)),
			wantESQueried:      false,
			wantRequeues:       []bool{false},
			wantEvaluation:     "",
		},
		{
			name:               "realm configured without requests: Elasticsearch not queried, no requeue",
			pkiRealmConfigured: true,
			esReachable:        true,
			nodeAttributes:     map[string]string{"es-0": "true", "es-1": ""},
			wantESQueried:      false,
			wantRequeues:       []bool{false},
		},
		{
			name:               "request already evaluated: Elasticsearch not queried, no requeue",
			pkiRealmConfigured: true,
			esReachable:        true,
			nodeAttributes:     map[string]string{"es-0": "true", "es-1": ""},
			request:            new(string(evaluated)),
			wantESQueried:      false,
			wantRequeues:       []bool{false},
			wantEvaluation:     string(evaluated),
		},
		{
			name:                    "request pending and realm running on all the nodes: evaluated, no requeue",
			pkiRealmConfigured:      true,
			esReachable:             true,
			nodeAttributes:          map[string]string{"es-0": "true", "es-1": "true"},
			request:                 new(""),
			wantESQueried:           true,
			wantClusterStateQueried: true,
			wantRequeues:            []bool{false},
			wantEvaluation:          string(evaluated),
		},
		{
			name:                    "request pending, realm running on all the nodes and a later file-based settings version applied: evaluated, no requeue",
			pkiRealmConfigured:      true,
			esReachable:             true,
			nodeAttributes:          map[string]string{"es-0": "true", "es-1": "true"},
			request:                 new(""),
			fileSettings:            func(persisted int64) esclient.FileSettings { return esclient.FileSettings{Version: persisted + 1} },
			wantESQueried:           true,
			wantClusterStateQueried: true,
			wantRequeues:            []bool{false},
			wantEvaluation:          string(evaluated),
		},
		{
			name:                    "request pending, realm running on all the nodes and file-based settings not applied yet: not evaluated, requeue",
			pkiRealmConfigured:      true,
			esReachable:             true,
			nodeAttributes:          map[string]string{"es-0": "true", "es-1": "true"},
			request:                 new(""),
			fileSettings:            func(persisted int64) esclient.FileSettings { return esclient.FileSettings{Version: persisted - 1} },
			wantESQueried:           true,
			wantClusterStateQueried: true,
			wantRequeues:            []bool{true},
		},
		{
			name:               "request pending, realm running on all the nodes and file-based settings failed to apply: not evaluated, requeue",
			pkiRealmConfigured: true,
			esReachable:        true,
			nodeAttributes:     map[string]string{"es-0": "true", "es-1": "true"},
			request:            new(""),
			fileSettings: func(persisted int64) esclient.FileSettings {
				return esclient.FileSettings{
					Version: persisted - 1,
					Errors:  &esclient.FileSettingsErrors{Version: persisted, ErrorKind: "validation", Errors: []string{"invalid"}},
				}
			},
			wantESQueried:           true,
			wantClusterStateQueried: true,
			wantRequeues:            []bool{true},
		},
		{
			name:                    "request pending, realm running on all the nodes and the cluster state request fails once: requeue, then evaluated",
			pkiRealmConfigured:      true,
			esReachable:             true,
			nodeAttributes:          map[string]string{"es-0": "true", "es-1": "true"},
			request:                 new(""),
			clusterStateErrs:        []error{errors.New("boom")},
			wantESQueried:           true,
			wantClusterStateQueried: true,
			wantRequeues:            []bool{true, false},
			wantEvaluation:          string(evaluated),
		},
		{
			name:               "request pending and realm not running on all the nodes: not evaluated, requeue",
			pkiRealmConfigured: true,
			esReachable:        true,
			nodeAttributes:     map[string]string{"es-0": "true", "es-1": ""},
			request:            new(""),
			wantESQueried:      true,
			wantRequeues:       []bool{true},
		},
		{
			name:               "conflict appears while the realm does not run on all the nodes: annotated without querying Elasticsearch",
			pkiRealmConfigured: true,
			esReachable:        true,
			nodeAttributes:     map[string]string{"es-0": "true", "es-1": ""},
			request:            new(string(evaluated)),
			conflicting:        true,
			wantESQueried:      false,
			wantRequeues:       []bool{false},
			wantConflict:       true,
		},
		{
			name:               "request pending and Elasticsearch not reachable: not evaluated, requeue",
			pkiRealmConfigured: true,
			esReachable:        false,
			request:            new(""),
			wantESQueried:      false,
			wantRequeues:       []bool{true},
		},
		{
			name:               "request pending and realm not running on all the nodes with orchestration paused: not evaluated, requeue",
			pkiRealmConfigured: true,
			paused:             true,
			esReachable:        true,
			nodeAttributes:     map[string]string{"es-0": "true", "es-1": ""},
			request:            new(""),
			wantESQueried:      true,
			wantRequeues:       []bool{true},
		},
		{
			name:                    "request pending and realm running on all the nodes with orchestration paused: evaluated, no requeue",
			pkiRealmConfigured:      true,
			paused:                  true,
			esReachable:             true,
			nodeAttributes:          map[string]string{"es-0": "true", "es-1": "true"},
			request:                 new(""),
			wantESQueried:           true,
			wantClusterStateQueried: true,
			wantRequeues:            []bool{false},
			wantEvaluation:          string(evaluated),
		},
		{
			name:               "request pending and Elasticsearch not reachable with orchestration paused: not evaluated, requeue",
			pkiRealmConfigured: true,
			paused:             true,
			esReachable:        false,
			request:            new(""),
			wantESQueried:      false,
			wantRequeues:       []bool{true},
		},
		{
			name:                    "request pending and the nodes request fails once with orchestration paused: requeue, then evaluated",
			pkiRealmConfigured:      true,
			paused:                  true,
			esReachable:             true,
			nodeAttributes:          map[string]string{"es-0": "true", "es-1": "true"},
			getNodesErrs:            []error{errors.New("boom")},
			request:                 new(""),
			wantESQueried:           true,
			wantClusterStateQueried: true,
			wantRequeues:            []bool{true, false},
			wantEvaluation:          string(evaluated),
		},
		{
			name:                    "request pending and a Pod missing from the nodes once with orchestration paused: requeue, then evaluated",
			pkiRealmConfigured:      true,
			paused:                  true,
			esReachable:             true,
			nodeAttributes:          map[string]string{"es-0": "true", "es-1": "true"},
			earlierNodeAttributes:   []map[string]string{{"es-0": "true"}},
			request:                 new(""),
			wantESQueried:           true,
			wantClusterStateQueried: true,
			wantRequeues:            []bool{true, false},
			wantEvaluation:          string(evaluated),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := es.DeepCopy()
			if tt.paused {
				es.Annotations = map[string]string{common.PauseOrchestrationAnnotation: "true"}
			}
			objs := []client.Object{es}
			if tt.request != nil {
				objs = append(objs, pkiRealmTestRequest(*es, *tt.request))
			}
			if tt.conflicting {
				other := pkiRealmTestRequest(*es, "")
				other.Name = "other-client-cert"
				other.Data[rolemapping.RolesField] = []byte("superuser")
				other.Data[rolemapping.ResourceUIDField] = []byte("uid-other")
				objs = append(objs, other)
			}
			c := k8s.NewFakeClient(objs...)
			esClient := &fakePKIRealmClient{
				nodeAttributes:        tt.nodeAttributes,
				earlierNodeAttributes: tt.earlierNodeAttributes,
				errs:                  tt.getNodesErrs,
				k8sClient:             c,
				es:                    k8s.ExtractNamespacedName(es),
				fileSettings:          tt.fileSettings,
				clusterStateErrs:      tt.clusterStateErrs,
			}
			sharedState := &shared.ReconcileState{
				ESReachable:    tt.esReachable,
				ESClient:       esClient,
				ResourcesState: &reconcile.ResourcesState{AllPods: pods},
			}
			requeues := make([]bool, 0, len(tt.wantRequeues))
			for range tt.wantRequeues {
				results := reconcilePKIRoleMappings(t.Context(), c, *es, tt.pkiRealmConfigured, sharedState)
				require.False(t, results.HasError())
				requeues = append(requeues, results.HasRequeue())
			}
			require.Equal(t, tt.wantRequeues, requeues)
			require.Equal(t, tt.wantESQueried, esClient.queried)
			require.Equal(t, tt.wantClusterStateQueried, esClient.clusterStateQueried)
			if tt.request != nil {
				var request corev1.Secret
				require.NoError(t, c.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: "kb-client-cert"}, &request))
				if tt.wantConflict {
					evaluation, ok := rolemapping.ParseEvaluation(request.Annotations[rolemapping.PKIEvaluationAnnotation])
					require.True(t, ok)
					require.NotEmpty(t, evaluation.Conflict)
					return
				}
				require.Equal(t, tt.wantEvaluation, request.Annotations[rolemapping.PKIEvaluationAnnotation])
			}
		})
	}
}
