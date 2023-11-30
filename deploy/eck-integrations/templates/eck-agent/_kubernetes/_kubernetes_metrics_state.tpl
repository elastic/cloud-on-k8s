{{/*
Config input for kubelet metrics
*/}}
{{- define "kubernetes.config.kube_state.input" -}}
{{- $metricSet := (list) }}
{{- $metricSet = append $metricSet (default .containers.state.enabled false) -}}
{{- $metricSet = append $metricSet (default .objects.cronjobs.state.enabled false) -}}
{{- $metricSet = append $metricSet (default .objects.daemonsets.state.enabled false) -}}
{{- $metricSet = append $metricSet (default .objects.deployments.state.enabled false) -}}
{{- $metricSet = append $metricSet (default .objects.jobs.state.enabled false) -}}
{{- $metricSet = append $metricSet (default .objects.nodes.state.enabled false) -}}
{{- $metricSet = append $metricSet (default .objects.persistentvolumes.state.enabled false) -}}
{{- $metricSet = append $metricSet (default .objects.persistentvolumeclaims.state.enabled false) -}}
{{- $metricSet = append $metricSet (default .objects.pods.state.enabled false) -}}
{{- $metricSet = append $metricSet (default .objects.replicasets.state.enabled false) -}}
{{- $metricSet = append $metricSet (default .objects.resourcequotas.state.enabled false) -}}
{{- $metricSet = append $metricSet (default .objects.services.state.enabled false) -}}
{{- $metricSet = append $metricSet (default .objects.statefulsets.state.enabled false) -}}
{{- $metricSet = append $metricSet (default .objects.storageclasses.state.enabled false) -}}
{{- if has true $metricSet -}}
- id: kubernetes/metrics-kube-state-metrics
  revision: 1
  name: kubernetes
  type: kubernetes/metrics
  data_stream:
      namespace: {{ .namespace }}
  use_output: default
  package_policy_id: {{ .integrationID | quote }}
  streams:
{{- if default .containers.state.enabled false }}
  - id: kubernetes/metrics-kubernetes.state_container
    data_stream:
      type: metrics
      dataset: kubernetes.state_container
    metricsets:
      - state_container
{{- include "kubernetes.config.kube_state.defaults" .containers.state | nindent 4 -}}
{{- end }}
{{- if default .objects.cronjobs.state.enabled false }}
  - id: kubernetes/metrics-kubernetes.state_cronjob
    data_stream:
      type: metrics
      dataset: kubernetes.state_cronjob
    metricsets:
      - state_cronjob
{{- include "kubernetes.config.kube_state.defaults" .objects.cronjobs.state | nindent 4 -}}
{{- end }}
{{- if default .objects.daemonsets.state.enabled false }}
  - id: kubernetes/metrics-kubernetes.state_daemonset
    data_stream:
      type: metrics
      dataset: kubernetes.state_daemonset
    metricsets:
      - state_daemonset
{{- include "kubernetes.config.kube_state.defaults" .objects.daemonsets.state | nindent 4 -}}
{{- end }}
{{- if default .objects.deployments.state.enabled false }}
  - id: kubernetes/metrics-kubernetes.state_deployment
    data_stream:
      type: metrics
      dataset: kubernetes.state_deployment
    metricsets:
      - state_deployment
{{- include "kubernetes.config.kube_state.defaults" .objects.deployments.state | nindent 4 -}}
{{- end }}
{{- if default .objects.jobs.state.enabled false }}
  - id: kubernetes/metrics-kubernetes.state_job
    data_stream:
      type: metrics
      dataset: kubernetes.state_job
    metricsets:
      - state_job
{{- include "kubernetes.config.kube_state.defaults" .objects.jobs.state | nindent 4 -}}
{{- end }}
{{- if default .objects.nodes.state.enabled false }}
  - id: kubernetes/metrics-kubernetes.state_node
    data_stream:
      type: metrics
      dataset: kubernetes.state_node
    metricsets:
      - state_node
{{- include "kubernetes.config.kube_state.defaults" .objects.nodes.state | nindent 4 -}}
{{- end }}
{{- if default .objects.persistentvolumes.state.enabled false }}
  - id: kubernetes/metrics-kubernetes.state_persistentvolume
    data_stream:
      type: metrics
      dataset: kubernetes.state_persistentvolume
    metricsets:
      - state_persistentvolume
{{- include "kubernetes.config.kube_state.defaults" .objects.persistentvolumes.state | nindent 4 -}}
{{- end }}
{{- if default .objects.persistentvolumeclaims.state.enabled false }}
  - id: kubernetes/metrics-kubernetes.state_persistentvolumeclaim
    data_stream:
      type: metrics
      dataset: kubernetes.state_persistentvolumeclaim
    metricsets:
      - state_persistentvolumeclaim
{{- include "kubernetes.config.kube_state.defaults" .objects.persistentvolumeclaims.state | nindent 4 -}}
{{- end }}
{{- if default .objects.pods.state.enabled false }}
  - id: kubernetes/metrics-kubernetes.state_pod
    data_stream:
      type: metrics
      dataset: kubernetes.state_pod
    metricsets:
      - state_pod
{{- include "kubernetes.config.kube_state.defaults" .objects.pods.state | nindent 4 -}}
{{- end }}
{{- if default .objects.replicasets.state.enabled false }}
  - id: kubernetes/metrics-kubernetes.state_replicaset
    data_stream:
      type: metrics
      dataset: kubernetes.state_replicaset
    metricsets:
      - state_replicaset
{{- include "kubernetes.config.kube_state.defaults" .objects.replicasets.state | nindent 4 -}}
{{- end }}
{{- if default .objects.resourcequotas.state.enabled false }}
  - id: kubernetes/metrics-kubernetes.state_resourcequota
    data_stream:
      type: metrics
      dataset: kubernetes.state_resourcequota
    metricsets:
      - state_resourcequota
{{- include "kubernetes.config.kube_state.defaults" .objects.resourcequotas.state | nindent 4 -}}
{{- end }}
{{- if default .objects.services.state.enabled false }}
  - id: kubernetes/metrics-kubernetes.state_service
    data_stream:
      type: metrics
      dataset: kubernetes.state_service
    metricsets:
      - state_service
{{- include "kubernetes.config.kube_state.defaults" .objects.services.state | nindent 4 -}}
{{- end }}
{{- if default .objects.statefulsets.state.enabled false }}
  - id: kubernetes/metrics-kubernetes.state_statefulset
    data_stream:
      type: metrics
      dataset: kubernetes.state_statefulset
    metricsets:
      - state_statefulset
{{- include "kubernetes.config.kube_state.defaults" .objects.statefulsets.state | nindent 4 -}}
{{- end }}
{{- if default .objects.storageclasses.state.enabled false }}
  - id: kubernetes/metrics-kubernetes.state_storageclass
    data_stream:
      type: metrics
      dataset: kubernetes.state_storageclass
    metricsets:
      - state_storageclass
{{- include "kubernetes.config.kube_state.defaults" .objects.storageclasses.state | nindent 4 -}}
{{- end }}
  meta:
    package:
      name: kubernetes
      version: {{ .version }}
{{- end -}}
{{- end -}}


{{/*
Defaults for kube_state input streams
*/}}
{{- define "kubernetes.config.kube_state.defaults" -}}
add_metadata: {{ dig "vars" "add_metadata" true . }}
hosts:
{{- range dig "vars" "hosts" (list "localhost:8080") . }}
- {{. | quote}}
{{- end }}
period: {{ dig "vars" "period" "10s" . }}
bearer_token_file: {{ dig "vars" "bearer_token_file" "/var/run/secrets/kubernetes.io/serviceaccount/token" .}}
{{- end -}}