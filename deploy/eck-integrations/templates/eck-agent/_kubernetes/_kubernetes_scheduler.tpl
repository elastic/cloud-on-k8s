{{- define "elasticagent.kubernetes.config.kube_scheduler.init" -}}
{{- if eq $.Values.kubernetes.scheduler.enabled true -}}
{{- $preset := $.Values.agent.presets.perNode -}}
{{- $inputVal := (include "elasticagent.kubernetes.config.kube_scheduler.input" $ | fromYamlArray) -}}
{{- include "elasticagent.preset.mutate.inputs" (list $ $preset $inputVal) -}}
{{- include "elasticagent.preset.applyOnce" (list $ $preset "elasticagent.kubernetes.pernode.preset") -}}
{{- end -}}
{{- end -}}

{{/*
Config input for kube_scheduler
*/}}
{{- define "elasticagent.kubernetes.config.kube_scheduler.input" -}}
- id: kubernetes/metrics-kubernetes.scheduler
  type: kubernetes/metrics
  data_stream:
    namespace: {{ .Values.kubernetes.namespace }}
  use_output: {{ .Values.kubernetes.output }}
  streams:
  - id: kubernetes/metrics-kubernetes.scheduler
    data_stream:
      type: metrics
      dataset: kubernetes.scheduler
    metricsets:
      - scheduler
{{- $vars := (include "elasticagent.kubernetes.config.kube_scheduler.default_vars" .) | fromYaml -}}
{{- mergeOverwrite $vars .Values.kubernetes.scheduler.vars | toYaml | nindent 4 }}
{{- end -}}


{{/*
Defaults for kube_scheduler input streams
*/}}
{{- define "elasticagent.kubernetes.config.kube_scheduler.default_vars" -}}
hosts:
 - "https://0.0.0.0:10259"
period: "10s"
bearer_token_file: "/var/run/secrets/kubernetes.io/serviceaccount/token"
ssl.verification_mode: "none"
condition: "${kubernetes.labels.component} == ''kube-scheduler''"
{{- end -}}