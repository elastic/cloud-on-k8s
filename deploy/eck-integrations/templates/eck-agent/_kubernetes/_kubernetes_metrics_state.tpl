{{/*
Config input for kubelet metrics
*/}}
{{- define "elasticagent.kubernetes.config.kube_state.init" -}}
{{- if eq ((.Values.kubernetes.state).enabled) true -}}
{{- $disabled := true -}}
{{- if and $disabled .Values.kubernetes.containers.state.enabled true -}}
{{- $disabled = false -}}
{{- end -}}
{{- if and $disabled .Values.kubernetes.cronjobs.state.enabled true -}}
{{- $disabled = false -}}
{{- end -}}
{{- if and $disabled .Values.kubernetes.daemonsets.state.enabled true -}}
{{- $disabled = false -}}
{{- end -}}
{{- if and $disabled .Values.kubernetes.deployments.state.enabled true -}}
{{- $disabled = false -}}
{{- end -}}
{{- if and $disabled .Values.kubernetes.jobs.state.enabled true -}}
{{- $disabled = false -}}
{{- end -}}
{{- if and $disabled .Values.kubernetes.nodes.state.enabled true -}}
{{- $disabled = false -}}
{{- end -}}
{{- if and $disabled .Values.kubernetes.persistentvolumes.state.enabled true -}}
{{- $disabled = false -}}
{{- end -}}
{{- if and $disabled .Values.kubernetes.persistentvolumeclaims.state.enabled true -}}
{{- $disabled = false -}}
{{- end -}}
{{- if and $disabled .Values.kubernetes.pods.state.enabled true -}}
{{- $disabled = false -}}
{{- end -}}
{{- if and $disabled .Values.kubernetes.replicasets.state.enabled true -}}
{{- $disabled = false -}}
{{- end -}}
{{- if and $disabled .Values.kubernetes.resourcequotas.state.enabled true -}}
{{- $disabled = false -}}
{{- end -}}
{{- if and $disabled .Values.kubernetes.services.state.enabled true -}}
{{- $disabled = false -}}
{{- end -}}
{{- if and $disabled .Values.kubernetes.statefulsets.state.enabled true -}}
{{- $disabled = false -}}
{{- end -}}
{{- if and $disabled .Values.kubernetes.storageclasses.state.enabled true -}}
{{- $disabled = false -}}
{{- end -}}
{{- if not $disabled -}}
{{- $preset := $.Values.eck_agent.presets.clusterWide -}}
{{- $inputVal := (include "elasticagent.kubernetes.config.kube_state.input" $ | fromYamlArray) -}}
{{- include "elasticagent.preset.mutate.inputs" (list $ $preset $inputVal) -}}
{{- include "elasticagent.preset.applyOnce" (list $ $preset "elasticagent.kubernetes.clusterwide.preset") -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "elasticagent.kubernetes.config.kube_state.input" -}}
{{- $vars := (include "elasticagent.kubernetes.config.kube_state.default_vars" .) | fromYaml -}}
{{- $vars = mergeOverwrite $vars .Values.kubernetes.state.vars -}}
- id: kubernetes/metrics-kube-state-metrics
  type: kubernetes/metrics
  data_stream:
      namespace: {{ .Values.kubernetes.namespace }}
  use_output: {{ .Values.kubernetes.output }}
  streams:
{{- if default false .Values.kubernetes.containers.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_container
    data_stream:
      type: metrics
      dataset: kubernetes.state_container
    metricsets:
      - state_container
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.containers.state.vars | toYaml | nindent 4 -}}
{{- end }}
{{- if default false .Values.kubernetes.cronjobs.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_cronjob
    data_stream:
      type: metrics
      dataset: kubernetes.state_cronjob
    metricsets:
      - state_cronjob
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.cronjobs.state.vars | toYaml | nindent 4 -}}
{{- end }}
{{- if default false .Values.kubernetes.daemonsets.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_daemonset
    data_stream:
      type: metrics
      dataset: kubernetes.state_daemonset
    metricsets:
      - state_daemonset
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.daemonsets.state.vars | toYaml | nindent 4 -}}
{{- end }}
{{- if default false .Values.kubernetes.deployments.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_deployment
    data_stream:
      type: metrics
      dataset: kubernetes.state_deployment
    metricsets:
      - state_deployment
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.deployments.state.vars | toYaml | nindent 4 -}}
{{- end }}
{{- if default false .Values.kubernetes.jobs.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_job
    data_stream:
      type: metrics
      dataset: kubernetes.state_job
    metricsets:
      - state_job
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.jobs.state.vars | toYaml | nindent 4 -}}
{{- end }}
{{- if default false .Values.kubernetes.nodes.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_node
    data_stream:
      type: metrics
      dataset: kubernetes.state_node
    metricsets:
      - state_node
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.nodes.state.vars | toYaml | nindent 4 -}}
{{- end }}
{{- if default false .Values.kubernetes.persistentvolumes.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_persistentvolume
    data_stream:
      type: metrics
      dataset: kubernetes.state_persistentvolume
    metricsets:
      - state_persistentvolume
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.persistentvolumes.state.vars | toYaml | nindent 4 -}}
{{- end }}
{{- if default false .Values.kubernetes.persistentvolumeclaims.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_persistentvolumeclaim
    data_stream:
      type: metrics
      dataset: kubernetes.state_persistentvolumeclaim
    metricsets:
      - state_persistentvolumeclaim
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.persistentvolumeclaims.state.vars | toYaml | nindent 4 -}}
{{- end }}
{{- if default false .Values.kubernetes.pods.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_pod
    data_stream:
      type: metrics
      dataset: kubernetes.state_pod
    metricsets:
      - state_pod
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.pods.state.vars | toYaml | nindent 4 -}}
{{- end }}
{{- if default false .Values.kubernetes.replicasets.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_replicaset
    data_stream:
      type: metrics
      dataset: kubernetes.state_replicaset
    metricsets:
      - state_replicaset
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.replicasets.state.vars | toYaml | nindent 4 -}}
{{- end }}
{{- if default false .Values.kubernetes.resourcequotas.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_resourcequota
    data_stream:
      type: metrics
      dataset: kubernetes.state_resourcequota
    metricsets:
      - state_resourcequota
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.resourcequotas.state.vars | toYaml | nindent 4 -}}
{{- end }}
{{- if default false .Values.kubernetes.services.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_service
    data_stream:
      type: metrics
      dataset: kubernetes.state_service
    metricsets:
      - state_service
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.services.state.vars | toYaml | nindent 4 -}}
{{- end }}
{{- if default false .Values.kubernetes.statefulsets.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_statefulset
    data_stream:
      type: metrics
      dataset: kubernetes.state_statefulset
    metricsets:
      - state_statefulset
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.statefulsets.state.vars | toYaml | nindent 4 -}}
{{- end }}
{{- if default false .Values.kubernetes.storageclasses.state.enabled }}
  - id: kubernetes/metrics-kubernetes.state_storageclass
    data_stream:
      type: metrics
      dataset: kubernetes.state_storageclass
    metricsets:
      - state_storageclass
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.storageclasses.state.vars | toYaml | nindent 4 -}}
{{- end }}
  meta:
    package:
      name: kubernetes
      version: {{ .Values.kubernetes.version }}
{{- end -}}


{{/*
Defaults for kube_state input streams
*/}}
{{- define "elasticagent.kubernetes.config.kube_state.default_vars" -}}
add_metadata: true
hosts:
- "localhost:8080"
period: "10s"
bearer_token_file: "/var/run/secrets/kubernetes.io/serviceaccount/token"
{{- end -}}