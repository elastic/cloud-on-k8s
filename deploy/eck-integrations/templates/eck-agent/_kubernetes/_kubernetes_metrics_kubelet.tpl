{{- define "agent.kubernetes.config.kubelet.enabled" -}}
{{- if eq ((.Values.kubernetes.metrics).enabled) false -}}
enabled: false
{{- $_ := set $.Values.kubernetes.containers.metrics "enabled" false -}}
{{- $_ := set $.Values.kubernetes.nodes.metrics "enabled" false -}}
{{- $_ := set $.Values.kubernetes.pods.metrics "enabled" false -}}
{{- $_ := set $.Values.kubernetes.volumes.metrics "enabled" false -}}
{{- $_ := set $.Values.kubernetes.system.metrics "enabled" false -}}
{{- else -}}
{{- $enabledInputs := (list) -}}
{{- $enabledInputs = append $enabledInputs (default false .Values.kubernetes.containers.metrics.enabled) -}}
{{- $enabledInputs = append $enabledInputs (default false .Values.kubernetes.nodes.metrics.enabled) -}}
{{- $enabledInputs = append $enabledInputs (default false .Values.kubernetes.pods.metrics.enabled) -}}
{{- $enabledInputs = append $enabledInputs (default false .Values.kubernetes.volumes.metrics.enabled) -}}
{{- $enabledInputs = append $enabledInputs (default false .Values.kubernetes.system.metrics.enabled) -}}
{{- if empty $enabledInputs }}
enabled: false
{{- else -}}
enabled: true
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Config input for kubelet metrics
*/}}
{{- define "agent.kubernetes.config.kubelet.input" -}}
{{- $vars := (include "agent.kubernetes.config.kubelet.default_vars" .) | fromYaml -}}
{{- $vars = mergeOverwrite $vars .Values.kubernetes.metrics.vars -}}
- id: kubernetes/metrics-kubelet
  type: kubernetes/metrics
  data_stream:
      namespace: {{ .Values.kubernetes.namespace }}
  use_output: default
  streams:
{{- if default false .Values.kubernetes.containers.metrics.enabled }}
  - id: kubernetes/metrics-kubernetes.container
    data_stream:
      type: metrics
      dataset: kubernetes.container
    metricsets:
      - container
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.containers.metrics.vars | toYaml | nindent 4 -}}
{{- end -}}
{{- if default false .Values.kubernetes.nodes.metrics.enabled }}
  - id: kubernetes/metrics-kubernetes.node
    data_stream:
      type: metrics
      dataset: kubernetes.node
    metricsets:
      - node
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.nodes.metrics.vars | toYaml | nindent 4 -}}
{{- end -}}
{{- if default false .Values.kubernetes.pods.metrics.enabled }}
  - id: kubernetes/metrics-kubernetes.pod
    data_stream:
      type: metrics
      dataset: kubernetes.pod
    metricsets:
      - pod
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.pods.metrics.vars | toYaml | nindent 4 -}}
{{- end -}}
{{- if default false .Values.kubernetes.volumes.metrics.enabled }}
  - id: kubernetes/metrics-kubernetes.volume
    data_stream:
      type: metrics
      dataset: kubernetes.volume
    metricsets:
      - volume
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.volumes.metrics.vars | toYaml | nindent 4 -}}
{{- end -}}
{{- if default false .Values.kubernetes.system.metrics.enabled }}
  - id: kubernetes/metrics-kubernetes.system
    data_stream:
      type: metrics
      dataset: kubernetes.system
    metricsets:
      - system
{{- mergeOverwrite (deepCopy $vars) .Values.kubernetes.system.metrics.vars | toYaml | nindent 4 -}}
{{- end }}
  meta:
    package:
      name: kubernetes
      version: {{.Values.kubernetes.version}}
{{- end -}}

{{/*
Defaults for kubelet input streams
*/}}
{{- define "agent.kubernetes.config.kubelet.default_vars" -}}
add_metadata: true
hosts:
- "https://${env.NODE_NAME}:10250"
period: "10s"
bearer_token_file: "/var/run/secrets/kubernetes.io/serviceaccount/token"
ssl.verification_mode: "none"
{{- end -}}