{{- define "agent.kubernetes.config.kube_scheduler.enabled" -}}
enabled: {{ .Values.kubernetes.scheduler.enabled }}
{{- end -}}

{{/*
Config input for kube_scheduler
*/}}
{{- define "agent.kubernetes.config.kube_scheduler.input" -}}
{{- $vars := (include "agent.kubernetes.config.kube_scheduler.default_vars" .) | fromYaml -}}
- id: kubernetes/metrics-kube-scheduler
  type: kubernetes/metrics
  data_stream:
    namespace: {{ .Values.kubernetes.namespace }}
  use_output: default
  streams:
    - id: kubernetes/metrics-kubernetes.scheduler
      data_stream:
        type: metrics
        dataset: kubernetes.scheduler
      metricsets:
        - scheduler
{{- mergeOverwrite $vars .Values.kubernetes.scheduler.vars | toYaml | nindent 4 }}
  meta:
    package:
      name: kubernetes
      version: {{ .Values.kubernetes.version }}
{{- end -}}


{{/*
Defaults for kube_scheduler input streams
*/}}
{{- define "agent.kubernetes.config.kube_scheduler.default_vars" -}}
hosts:
 - "https://0.0.0.0:10259"
period: "10s"
bearer_token_file: "/var/run/secrets/kubernetes.io/serviceaccount/token"
ssl.verification_mode: "none"
condition: "${kubernetes.labels.component} == ''kube-scheduler''"
{{- end -}}