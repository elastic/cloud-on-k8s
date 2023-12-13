{{/*
Config input for kubelet metrics
*/}}
{{- define "agent.kubernetes.config.kubelet.input" -}}
{{- $metricSet := (list) }}
{{- $metricSet = append $metricSet (default false .containers.metrics.enabled) -}}
{{- $metricSet = append $metricSet (default false .nodes.metrics.enabled) -}}
{{- $metricSet = append $metricSet (default false .pods.metrics.enabled) -}}
{{- $metricSet = append $metricSet (default false .volumes.metrics.enabled) -}}
{{- $metricSet = append $metricSet (default false .system.metrics.enabled) -}}
{{- if has true $metricSet -}}
- id: kubernetes/metrics-kubelet
  type: kubernetes/metrics
  data_stream:
      namespace: {{ .namespace }}
  use_output: default
  streams:
{{- if default false .containers.metrics.enabled }}
  - id: kubernetes/metrics-kubernetes.container
    data_stream:
      type: metrics
      dataset: kubernetes.container
    metricsets:
      - container
{{- include "agent.kubernetes.config.kubelet.defaults" .containers.metrics | nindent 4 -}}
{{- end -}}
{{- if default false .nodes.metrics.enabled }}
  - id: kubernetes/metrics-kubernetes.node
    data_stream:
      type: metrics
      dataset: kubernetes.node
    metricsets:
      - node
{{- include "agent.kubernetes.config.kubelet.defaults" .nodes.metrics | nindent 4 -}}
{{- end -}}
{{- if default false .pods.metrics.enabled }}
  - id: kubernetes/metrics-kubernetes.pod
    data_stream:
      type: metrics
      dataset: kubernetes.pod
    metricsets:
      - pod
{{- include "agent.kubernetes.config.kubelet.defaults" .pods.metrics | nindent 4 -}}
{{- end -}}
{{- if default false .volumes.metrics.enabled }}
  - id: kubernetes/metrics-kubernetes.volume
    data_stream:
      type: metrics
      dataset: kubernetes.volume
    metricsets:
      - volume
{{- include "agent.kubernetes.config.kubelet.defaults" .volumes.metrics | nindent 4 -}}
{{- end -}}
{{- if default false .system.metrics.enabled }}
  - id: kubernetes/metrics-kubernetes.system
    data_stream:
      type: metrics
      dataset: kubernetes.system
    metricsets:
      - system
{{- include "agent.kubernetes.config.kubelet.defaults" .system.metrics | nindent 4 -}}
{{- end }}
  meta:
    package:
      name: kubernetes
      version: {{.version}}
{{- end -}}
{{- end -}}

{{/*
Defaults for kubelet input streams
*/}}
{{- define "agent.kubernetes.config.kubelet.defaults" -}}
add_metadata: {{ dig "vars" "add_metadata" true . }}
hosts:
{{- range dig "vars" "hosts" (list "https://${env.NODE_NAME}:10250") . }}
- {{.}}
{{- end }}
period: {{ dig "vars" "period" "10s" . }}
bearer_token_file: {{ dig "vars" "bearer_token_file" "/var/run/secrets/kubernetes.io/serviceaccount/token" .}}
ssl.verification_mode: {{ dig "vars" "ssl.verification_mode" "none" . }}
{{- end -}}