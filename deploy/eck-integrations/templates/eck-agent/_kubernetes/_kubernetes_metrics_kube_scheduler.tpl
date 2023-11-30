{{/*
Config input for kube_scheduler
*/}}
{{- define "kubernetes.config.kube_scheduler.input" -}}
{{- if default .control_plane.scheduler.enabled false -}}
- id: kubernetes/metrics-kube-scheduler
  revision: 1
  name: kubernetes
  type: kubernetes/metrics
  data_stream:
    namespace: {{ .namespace }}
  use_output: default
  package_policy_id: {{.integrationID}}
  streams:
    - id: kubernetes/metrics-kubernetes.scheduler
      data_stream:
        type: metrics
        dataset: kubernetes.scheduler
      metricsets:
        - scheduler
{{- include "config.kube_scheduler.defaults" .control_plane.scheduler | nindent 4 }} 
  meta:
    package:
      name: kubernetes
      version: {{ .version }}
{{- end -}}
{{- end -}}


{{/*
Defaults for kube_scheduler input streams
*/}}
{{- define "config.kube_scheduler.defaults" -}}
hosts:
{{- range dig "vars" "hosts" (list "https://0.0.0.0:10259") . }}
- {{. | quote}}
{{- end }}
period: {{ dig "vars" "period" "10s" . }}
bearer_token_file: {{ dig "vars" "bearer_token_file" "/var/run/secrets/kubernetes.io/serviceaccount/token" .}}
ssl.verification_mode: {{ dig "vars" "ssl.verification_mode" "none" . }}
condition: {{ dig "vars" "condition" "${kubernetes.labels.component} == ''kube-scheduler''" . | quote }}
{{- end -}}