// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package restart

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type ExecutedSteps struct {
	steps []string
}

func podInPhase(phase Phase) corev1.Pod {
	var annotations map[string]string
	if phase != "" {
		annotations = map[string]string{PhaseAnnotation: string(phase)}
	}
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "ns",
			Name:        rand.String(8),
			Annotations: annotations,
		},
	}
}

var cluster = v1alpha1.Elasticsearch{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: "ns",
		Name:      "test-cluster",
	},
}

var esService = corev1.Service{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: "ns",
		Name:      services.ExternalServiceName(cluster.Name),
	},
}

var endpointsReady = corev1.Endpoints{
	ObjectMeta: esService.ObjectMeta,
	Subsets:    []corev1.EndpointSubset{{Addresses: []corev1.EndpointAddress{{}}}},
}

var endpointsNotReady = corev1.Endpoints{
	ObjectMeta: endpointsReady.ObjectMeta,
}

var esClientOK = client.NewMockClient(version.MustParse("6.7.0"), func(req *http.Request) *http.Response {
	return client.NewMockResponse(200, req, "")
})
var esClientKO = client.NewMockClient(version.MustParse("6.7.0"), func(req *http.Request) *http.Response {
	return client.NewMockResponse(500, req, "")
})

func TestCoordinatedRestart_coordinatedStepsExec(t *testing.T) {
	genStep := func(name string, initPhase Phase, endPhase Phase, executed *ExecutedSteps, moveToEndPhase bool, res bool, err error) Step {
		return Step{
			initPhase: initPhase,
			endPhase:  endPhase,
			do: func(pods []corev1.Pod) (b bool, e error) {
				for _, p := range pods {
					executed.steps = append(executed.steps, name)
					if moveToEndPhase {
						p.Annotations[PhaseAnnotation] = string(endPhase)
					}
				}
				return res, err
			},
		}
	}
	stepDone := func(name string, initPhase Phase, endPhase Phase, executed *ExecutedSteps, moveToEndPhase bool) Step {
		return genStep(name, initPhase, endPhase, executed, moveToEndPhase, true, nil)
	}
	stepNotDone := func(name string, initPhase Phase, endPhase Phase, executed *ExecutedSteps, moveToEndPhase bool) Step {
		return genStep(name, initPhase, endPhase, executed, moveToEndPhase, false, nil)
	}
	stepErr := func(name string, initPhase Phase, endPhase Phase, executed *ExecutedSteps, moveToEndPhase bool) Step {
		return genStep(name, initPhase, endPhase, executed, moveToEndPhase, false, errors.New("step fail"))
	}

	tests := []struct {
		name              string
		pods              []corev1.Pod
		Timeout           time.Duration
		steps             func(executedSteps *ExecutedSteps) []Step
		wantExecutedSteps []string
		wantDone          bool
		wantErr           error
	}{
		{
			name:     "no pods",
			pods:     nil,
			Timeout:  CoordinatedRestartDefaultTimeout,
			wantDone: true,
		},
		{
			name:     "no steps",
			pods:     []corev1.Pod{{}},
			Timeout:  CoordinatedRestartDefaultTimeout,
			wantDone: true,
		},
		{
			name: "pods not annotated with any phase",
			pods: []corev1.Pod{
				podInPhase(""),
				podInPhase(""),
			},
			steps: func(s *ExecutedSteps) []Step {
				return []Step{
					stepDone("1", PhaseSchedule, PhaseStop, s, false),
				}
			},
			Timeout:  CoordinatedRestartDefaultTimeout,
			wantDone: true,
		},
		{
			name: "2 pods should execute step 1",
			pods: []corev1.Pod{
				podInPhase(PhaseSchedule),
				podInPhase(PhaseSchedule),
			},
			steps: func(s *ExecutedSteps) []Step {
				return []Step{
					stepDone("1", PhaseSchedule, PhaseStop, s, false),
				}
			},
			wantExecutedSteps: []string{"1", "1"},
			Timeout:           CoordinatedRestartDefaultTimeout,
			wantDone:          false,
		},
		{
			name: "2 pods should execute step 1, 1 pod should do nothing in step 2 since step 1 not completed yet",
			pods: []corev1.Pod{
				podInPhase(PhaseSchedule),
				podInPhase(PhaseSchedule),
				podInPhase(PhaseStop),
			},
			steps: func(s *ExecutedSteps) []Step {
				return []Step{
					stepDone("1", PhaseSchedule, PhaseStop, s, false),
					stepDone("2", PhaseStop, PhaseStart, s, false),
				}
			},
			wantExecutedSteps: []string{"1", "1"},
			Timeout:           CoordinatedRestartDefaultTimeout,
			wantDone:          false,
		},
		{
			name: "step 1 over, 3 pods in step 2",
			pods: []corev1.Pod{
				podInPhase(PhaseStop),
				podInPhase(PhaseStop),
				podInPhase(PhaseStop),
			},
			steps: func(s *ExecutedSteps) []Step {
				return []Step{
					stepDone("1", PhaseSchedule, PhaseStop, s, false),
					stepDone("2", PhaseStop, PhaseStart, s, false),
				}
			},
			wantExecutedSteps: []string{"2", "2", "2"},
			Timeout:           CoordinatedRestartDefaultTimeout,
			wantDone:          false,
		},
		{
			name: "step error",
			pods: []corev1.Pod{
				podInPhase(PhaseSchedule),
				podInPhase(PhaseSchedule),
			},
			steps: func(s *ExecutedSteps) []Step {
				return []Step{
					stepErr("1", PhaseSchedule, PhaseStop, s, false),
				}
			},
			wantExecutedSteps: []string{"1", "1"},
			Timeout:           CoordinatedRestartDefaultTimeout,
			wantDone:          false,
			wantErr:           errors.New("step fail"),
		},
		{
			name: "step not done",
			pods: []corev1.Pod{
				podInPhase(PhaseSchedule),
				podInPhase(PhaseSchedule),
			},
			steps: func(s *ExecutedSteps) []Step {
				return []Step{
					stepNotDone("1", PhaseSchedule, PhaseStop, s, false),
				}
			},
			wantExecutedSteps: []string{"1", "1"},
			Timeout:           CoordinatedRestartDefaultTimeout,
			wantDone:          false,
		},
		{
			name: "pods progress from step 1 to step 2 to step 3",
			pods: []corev1.Pod{
				podInPhase(PhaseSchedule),
				podInPhase(PhaseSchedule),
			},
			steps: func(s *ExecutedSteps) []Step {
				return []Step{
					stepDone("1", PhaseSchedule, PhaseStop, s, true),
					stepDone("2", PhaseStop, PhaseStart, s, true),
					stepDone("3", PhaseStart, Phase(""), s, true),
				}
			},
			wantExecutedSteps: []string{"1", "1", "2", "2", "3", "3"},
			Timeout:           CoordinatedRestartDefaultTimeout,
			wantDone:          true,
		},
		{
			name: "second pod reached its restart timeout, only first pods executes steps",
			pods: []corev1.Pod{
				podInPhase(PhaseSchedule),
				{ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      rand.String(8),
					Annotations: map[string]string{
						PhaseAnnotation:     string(PhaseSchedule),
						StartTimeAnnotation: time.Now().Add(-1 * time.Minute).Format(time.RFC3339Nano),
					}}},
			},
			steps: func(s *ExecutedSteps) []Step {
				return []Step{
					stepDone("1", PhaseSchedule, PhaseStop, s, true),
					stepDone("2", PhaseStop, PhaseStart, s, true),
					stepDone("3", PhaseStart, Phase(""), s, true),
				}
			},
			wantExecutedSteps: []string{"1", "2", "3"},
			Timeout:           1 * time.Second,
			wantDone:          true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var runtimeObjs []runtime.Object
			for i := range tt.pods {
				runtimeObjs = append(runtimeObjs, &(tt.pods[i]))
			}
			k8sClient := k8s.WrapClient(fake.NewFakeClient(runtimeObjs...))
			c := CoordinatedRestart{
				RestartContext: RestartContext{
					K8sClient:      k8sClient,
					EventsRecorder: &events.Recorder{},
				},
				pods:    tt.pods,
				timeout: tt.Timeout,
			}
			executedSteps := ExecutedSteps{}
			if tt.steps == nil {
				tt.steps = func(executedSteps *ExecutedSteps) []Step { return nil }
			}
			done, err := c.coordinatedStepsExec(tt.steps(&executedSteps)...)
			assert.Equal(t, tt.wantDone, done)
			assert.Equal(t, tt.wantExecutedSteps, executedSteps.steps)
			if tt.wantErr != nil {
				assert.Error(t, tt.wantErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

type stepTest struct {
	step           func(c *CoordinatedRestart) Step
	name           string
	pods           []corev1.Pod
	esEndpoints    corev1.Endpoints
	expectedPhases []Phase
	esClient       client.Client
	pmClient       processmanager.Client
	wantDone       bool
	wantErr        bool
}

func runStepTests(t *testing.T, tests []stepTest) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtimeObjs := []runtime.Object{&esService, &tt.esEndpoints}
			for i := range tt.pods {
				runtimeObjs = append(runtimeObjs, &tt.pods[i])
			}
			k8sClient := k8s.WrapClient(fake.NewFakeClient(runtimeObjs...))
			c := CoordinatedRestart{
				RestartContext: RestartContext{
					K8sClient:      k8sClient,
					EventsRecorder: &events.Recorder{},
					EsClient:       tt.esClient,
					Cluster:        cluster,
				},
				pmClientFactory: func(restartContext RestartContext, pod corev1.Pod) (processmanager.Client, error) {
					return tt.pmClient, nil
				},
			}
			done, err := tt.step(&c).do(tt.pods)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantDone, done)
			var podsPhases []Phase
			for _, p := range tt.pods {
				var pod corev1.Pod
				err := k8sClient.Get(k8s.ExtractNamespacedName(&p), &pod)
				require.NoError(t, err)
				phase, _ := getPhase(pod)
				podsPhases = append(podsPhases, phase)
			}
			require.Equal(t, tt.expectedPhases, podsPhases)
		})
	}
}

func TestCoordinatedRestart_scheduleStop(t *testing.T) {
	step := func(c *CoordinatedRestart) Step { return c.scheduleStop() }
	tests := []stepTest{
		{
			name:           "happy path",
			step:           step,
			pods:           []corev1.Pod{podInPhase(PhaseSchedule), podInPhase(PhaseSchedule)},
			expectedPhases: []Phase{PhaseStop, PhaseStop},
			esClient:       esClientOK,
			wantDone:       true,
		},
		{
			name:           "es client error: continue (best effort cluster calls)",
			step:           step,
			pods:           []corev1.Pod{podInPhase(PhaseSchedule), podInPhase(PhaseSchedule)},
			expectedPhases: []Phase{PhaseStop, PhaseStop},
			esClient:       esClientKO,
			wantDone:       true,
			wantErr:        false,
		},
	}
	runStepTests(t, tests)
}

func TestCoordinatedRestart_stop(t *testing.T) {
	step := func(c *CoordinatedRestart) Step { return c.stop() }
	tests := []stepTest{
		{
			name:           "nodes are still started",
			step:           step,
			pods:           []corev1.Pod{podInPhase(PhaseStop), podInPhase(PhaseStop)},
			expectedPhases: []Phase{PhaseStop, PhaseStop},
			esClient:       esClientOK,
			pmClient:       processmanager.NewMockClient(processmanager.ProcessStatus{State: processmanager.Started}, nil),
			wantDone:       false,
			wantErr:        false,
		},
		{
			name:           "nodes are stopping",
			step:           step,
			pods:           []corev1.Pod{podInPhase(PhaseStop), podInPhase(PhaseStop)},
			expectedPhases: []Phase{PhaseStop, PhaseStop},
			esClient:       esClientOK,
			pmClient:       processmanager.NewMockClient(processmanager.ProcessStatus{State: processmanager.Stopping}, nil),
			wantDone:       false,
			wantErr:        false,
		},
		{
			name:           "nodes are stopped",
			step:           step,
			pods:           []corev1.Pod{podInPhase(PhaseStop), podInPhase(PhaseStop)},
			expectedPhases: []Phase{PhaseStart, PhaseStart},
			esClient:       esClientOK,
			pmClient:       processmanager.NewMockClient(processmanager.ProcessStatus{State: processmanager.Stopped}, nil),
			wantDone:       true,
			wantErr:        false,
		},
		{
			name:           "process manager cannot be reached",
			step:           step,
			pods:           []corev1.Pod{podInPhase(PhaseStop), podInPhase(PhaseStop)},
			expectedPhases: []Phase{PhaseStop, PhaseStop},
			esClient:       esClientOK,
			pmClient:       processmanager.NewMockClient(processmanager.ProcessStatus{State: processmanager.Stopped}, errors.New("failure")),
			wantDone:       false,
			wantErr:        true,
		},
	}
	runStepTests(t, tests)
}

func TestCoordinatedRestart_start(t *testing.T) {
	step := func(c *CoordinatedRestart) Step { return c.start() }
	tests := []stepTest{
		{
			name:           "nodes are still stopped",
			step:           step,
			pods:           []corev1.Pod{podInPhase(PhaseStart), podInPhase(PhaseStart)},
			expectedPhases: []Phase{PhaseStart, PhaseStart},
			esClient:       esClientOK,
			pmClient:       processmanager.NewMockClient(processmanager.ProcessStatus{State: processmanager.Stopped}, nil),
			wantDone:       false,
			wantErr:        false,
		},
		{
			name:           "nodes are started, but service is not ready yet",
			step:           step,
			pods:           []corev1.Pod{podInPhase(PhaseStart), podInPhase(PhaseStart)},
			esEndpoints:    endpointsNotReady,
			expectedPhases: []Phase{PhaseStart, PhaseStart},
			esClient:       esClientOK,
			pmClient:       processmanager.NewMockClient(processmanager.ProcessStatus{State: processmanager.Started}, nil),
			wantDone:       false,
			wantErr:        false,
		},
		{
			name:           "nodes are started, service is ready, cannot re-enable shards allocation",
			step:           step,
			pods:           []corev1.Pod{podInPhase(PhaseStart), podInPhase(PhaseStart)},
			esEndpoints:    endpointsReady,
			expectedPhases: []Phase{PhaseStart, PhaseStart},
			esClient:       esClientKO,
			pmClient:       processmanager.NewMockClient(processmanager.ProcessStatus{State: processmanager.Started}, nil),
			wantDone:       false,
			wantErr:        true,
		},
		{
			name:           "happy path",
			step:           step,
			pods:           []corev1.Pod{podInPhase(PhaseStart), podInPhase(PhaseStart)},
			esEndpoints:    endpointsReady,
			expectedPhases: []Phase{"", ""},
			esClient:       esClientOK,
			pmClient:       processmanager.NewMockClient(processmanager.ProcessStatus{State: processmanager.Started}, nil),
			wantDone:       true,
			wantErr:        false,
		},
	}
	runStepTests(t, tests)
}

func TestCoordinatedRestart_abortIfTimeoutReached(t *testing.T) {
	tests := []struct {
		name           string
		annotatedStart time.Time
		timeout        time.Duration
		want           bool
		wantErr        bool
		wantEvents     int
	}{
		{
			name:           "timeout not reached",
			annotatedStart: time.Now().Add(-1 * time.Minute),
			timeout:        CoordinatedRestartDefaultTimeout,
			want:           false,
			wantErr:        false,
		},
		{
			name:           "timeout reached",
			annotatedStart: time.Now().Add(-1 * time.Minute),
			timeout:        1 * time.Second,
			want:           true,
			wantErr:        false,
			wantEvents:     1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventsRecorder := events.NewRecorder()
			// create the pod, properly annotated
			pod := podInPhase(PhaseSchedule)
			c := &CoordinatedRestart{
				RestartContext: RestartContext{
					K8sClient:      k8s.WrapClient(fake.NewFakeClient(&pod)),
					EventsRecorder: eventsRecorder,
				},
				timeout: tt.timeout,
			}
			err := setScheduleRestartAnnotations(c.K8sClient, pod, StrategyCoordinated, tt.annotatedStart)
			require.NoError(t, err)

			// check timeout
			got, err := c.abortIfTimeoutReached(pod)
			if (err != nil) != tt.wantErr {
				t.Errorf("CoordinatedRestart.abortIfTimeoutReached() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("CoordinatedRestart.abortIfTimeoutReached() = %v, want %v", got, tt.want)
			}
			if tt.want {
				// annotations should be deleted
				var updatedPod corev1.Pod
				err = c.K8sClient.Get(k8s.ExtractNamespacedName(&pod), &updatedPod)
				require.NoError(t, err)
				require.Empty(t, updatedPod.Annotations[StartTimeAnnotation])
				require.Empty(t, updatedPod.Annotations[PhaseAnnotation])
				require.Empty(t, updatedPod.Annotations[StrategyAnnotation])
			}
			require.Equal(t, tt.wantEvents, len(eventsRecorder.Events()))
		})
	}
}
