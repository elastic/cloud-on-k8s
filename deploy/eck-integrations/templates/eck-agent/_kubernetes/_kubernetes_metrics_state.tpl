{{/*
Config input for kubelet metrics
*/}}
{{- define "agent.kubernetes.config.kube_state.input" -}}
{{- $metricSet := (list) }}
{{- $metricSet = append $metricSet (default false .containers.state.enabled) -}}
{{- $metricSet = append $metricSet (default false .objects.cronjobs.state.enabled) -}}
{{- $metricSet = append $metricSet (default false .objects.daemonsets.state.enabled) -}}
{{- $metricSet = append $metricSet (default false .objects.deployments.state.enabled) -}}
{{- $metricSet = append $metricSet (default false .objects.jobs.state.enabled) -}}
{{- $metricSet = append $metricSet (default false .objects.nodes.state.enabled) -}}
{{- $metricSet = append $metricSet (default false .objects.persistentvolumes.state.enabled) -}}
{{- $metricSet = append $metricSet (default false .objects.persistentvolumeclaims.state.enabled) -}}
{{- $metricSet = append $metricSet (default false .objects.pods.state.enabled) -}}
{{- $metricSet = append $metricSet (default false .objects.replicasets.state.enabled) -}}
{{- $metricSet = append $metricSet (default false .objects.resourcequotas.state.enabled) -}}
{{- $metricSet = append $metricSet (default false .objects.services.state.enabled) -}}
{{- $metricSet = append $metricSet (default false .objects.statefulsets.state.enabled) -}}
{{- $metricSet = append $metricSet (default false .objects.storageclasses.state.enabled) -}}
{{- if has true $metricSet -}}
- id: kubernetes/metrics-kube-state-metrics
  type: kubernetes/metrics
  data_stream:
      namespace: {{ .namespace }}
  use_output: default
  streams:
{{- if default false .containers.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_container
    data_stream:
      type: metrics
      dataset: kubernetes.state_container
    metricsets:
      - state_container
{{- include "agent.kubernetes.config.kube_state.defaults" .containers.state | nindent 4 -}}
{{- end }}
{{- if default false .objects.cronjobs.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_cronjob
    data_stream:
      type: metrics
      dataset: kubernetes.state_cronjob
    metricsets:
      - state_cronjob
{{- include "agent.kubernetes.config.kube_state.defaults" .objects.cronjobs.state | nindent 4 -}}
{{- end }}
{{- if default false .objects.daemonsets.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_daemonset
    data_stream:
      type: metrics
      dataset: kubernetes.state_daemonset
    metricsets:
      - state_daemonset
{{- include "agent.kubernetes.config.kube_state.defaults" .objects.daemonsets.state | nindent 4 -}}
{{- end }}
{{- if default false .objects.deployments.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_deployment
    data_stream:
      type: metrics
      dataset: kubernetes.state_deployment
    metricsets:
      - state_deployment
{{- include "agent.kubernetes.config.kube_state.defaults" .objects.deployments.state | nindent 4 -}}
{{- end }}
{{- if default false .objects.jobs.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_job
    data_stream:
      type: metrics
      dataset: kubernetes.state_job
    metricsets:
      - state_job
{{- include "agent.kubernetes.config.kube_state.defaults" .objects.jobs.state | nindent 4 -}}
{{- end }}
{{- if default false .objects.nodes.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_node
    data_stream:
      type: metrics
      dataset: kubernetes.state_node
    metricsets:
      - state_node
{{- include "agent.kubernetes.config.kube_state.defaults" .objects.nodes.state | nindent 4 -}}
{{- end }}
{{- if default false .objects.persistentvolumes.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_persistentvolume
    data_stream:
      type: metrics
      dataset: kubernetes.state_persistentvolume
    metricsets:
      - state_persistentvolume
{{- include "agent.kubernetes.config.kube_state.defaults" .objects.persistentvolumes.state | nindent 4 -}}
{{- end }}
{{- if default false .objects.persistentvolumeclaims.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_persistentvolumeclaim
    data_stream:
      type: metrics
      dataset: kubernetes.state_persistentvolumeclaim
    metricsets:
      - state_persistentvolumeclaim
{{- include "agent.kubernetes.config.kube_state.defaults" .objects.persistentvolumeclaims.state | nindent 4 -}}
{{- end }}
{{- if default false .objects.pods.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_pod
    data_stream:
      type: metrics
      dataset: kubernetes.state_pod
    metricsets:
      - state_pod
{{- include "agent.kubernetes.config.kube_state.defaults" .objects.pods.state | nindent 4 -}}
{{- end }}
{{- if default false .objects.replicasets.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_replicaset
    data_stream:
      type: metrics
      dataset: kubernetes.state_replicaset
    metricsets:
      - state_replicaset
{{- include "agent.kubernetes.config.kube_state.defaults" .objects.replicasets.state | nindent 4 -}}
{{- end }}
{{- if default false .objects.resourcequotas.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_resourcequota
    data_stream:
      type: metrics
      dataset: kubernetes.state_resourcequota
    metricsets:
      - state_resourcequota
{{- include "agent.kubernetes.config.kube_state.defaults" .objects.resourcequotas.state | nindent 4 -}}
{{- end }}
{{- if default false .objects.services.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_service
    data_stream:
      type: metrics
      dataset: kubernetes.state_service
    metricsets:
      - state_service
{{- include "agent.kubernetes.config.kube_state.defaults" .objects.services.state | nindent 4 -}}
{{- end }}
{{- if default false .objects.statefulsets.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_statefulset
    data_stream:
      type: metrics
      dataset: kubernetes.state_statefulset
    metricsets:
      - state_statefulset
{{- include "agent.kubernetes.config.kube_state.defaults" .objects.statefulsets.state | nindent 4 -}}
{{- end }}
{{- if default false .objects.storageclasses.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_storageclass
    data_stream:
      type: metrics
      dataset: kubernetes.state_storageclass
    metricsets:
      - state_storageclass
{{- include "agent.kubernetes.config.kube_state.defaults" .objects.storageclasses.state | nindent 4 -}}
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
{{- define "agent.kubernetes.config.kube_state.defaults" -}}
add_metadata: {{ dig "vars" "add_metadata" true . }}
hosts:
{{- range dig "vars" "hosts" (list "localhost:8080") . }}
- {{. | quote}}
{{- end }}
period: {{ dig "vars" "period" "10s" . }}
bearer_token_file: {{ dig "vars" "bearer_token_file" "/var/run/secrets/kubernetes.io/serviceaccount/token" .}}
{{- end -}}