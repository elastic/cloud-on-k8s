{{/*
Config input for kubelet metrics
*/}}
{{- define "kubernetes.config.kubelet.input" -}}
{{- $metricSet := (list) }}
{{- $metricSet = append $metricSet (default .containers.metrics.enabled false) -}}
{{- $metricSet = append $metricSet (default .objects.nodes.metrics.enabled false) -}}
{{- $metricSet = append $metricSet (default .objects.pods.metrics.enabled false) -}}
{{- $metricSet = append $metricSet (default .objects.volumes.metrics.enabled false) -}}
{{- $metricSet = append $metricSet (default .system.metrics.enabled false) -}}
{{- if has true $metricSet -}}
- id: kubernetes/metrics-kubelet
  revision: 1
  name: kubernetes
  type: kubernetes/metrics
  data_stream:
      namespace: {{ .namespace }}
  use_output: default
  package_policy_id: {{.integrationID}}
  streams:
{{- if default .containers.metrics.enabled false }}
  - id: kubernetes/metrics-kubernetes.container
    data_stream:
      type: metrics
      dataset: kubernetes.container
    metricsets:
      - container
{{- include "kubernetes.config.kubelet.defaults" .containers.metrics | nindent 4 -}}
{{- end -}}
{{- if default .objects.nodes.metrics.enabled false }}
  - id: kubernetes/metrics-kubernetes.node
    data_stream:
      type: metrics
      dataset: kubernetes.node
    metricsets:
      - node
{{- include "kubernetes.config.kubelet.defaults" .objects.nodes.metrics | nindent 4 -}}
{{- end -}}
{{- if default .objects.pods.metrics.enabled false }}
  - id: kubernetes/metrics-kubernetes.pod
    data_stream:
      type: metrics
      dataset: kubernetes.pod
    metricsets:
      - pod
{{- include "kubernetes.config.kubelet.defaults" .objects.pods.metrics | nindent 4 -}}
{{- end -}}
{{- if default .objects.volumes.metrics.enabled false }}
  - id: kubernetes/metrics-kubernetes.volume
    data_stream:
      type: metrics
      dataset: kubernetes.volume
    metricsets:
      - volume
{{- include "kubernetes.config.kubelet.defaults" .objects.volumes.metrics | nindent 4 -}}
{{- end -}}
{{- if default .system.metrics.enabled false }}
  - id: kubernetes/metrics-kubernetes.system
    data_stream:
      type: metrics
      dataset: kubernetes.system
    metricsets:
      - system
{{- include "kubernetes.config.kubelet.defaults" .system.metrics | nindent 4 -}}
{{- end }}
  meta:
    package:
      name: kubernetes
      version: .version
{{- end -}}
{{- end -}}

{{/*
Defaults for kubelet input streams
*/}}
{{- define "kubernetes.config.kubelet.defaults" -}}
add_metadata: {{ dig "vars" "add_metadata" true . }}
hosts:
{{- range dig "vars" "hosts" (list "https://${env.NODE_NAME}:10250") . }}
- {{.}}
{{- end }}
period: {{ dig "vars" "period" "10s" . }}
bearer_token_file: {{ dig "vars" "bearer_token_file" "/var/run/secrets/kubernetes.io/serviceaccount/token" .}}
ssl.verification_mode: {{ dig "vars" "ssl.verification_mode" "none" . }}
{{- end -}}