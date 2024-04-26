{{- define "elasticagent.kubernetes.config.kube_controller.init" -}}
{{- if eq $.Values.kubernetes.controller_manager.enabled true -}}
{{- $preset := $.Values.agent.presets.perNode -}}
{{- $inputVal := (include "elasticagent.kubernetes.config.kube_controller.input" $ | fromYamlArray) -}}
{{- include "elasticagent.preset.mutate.inputs" (list $ $preset $inputVal) -}}
{{- include "elasticagent.preset.applyOnce" (list $ $preset "elasticagent.kubernetes.pernode.preset") -}}
{{- end -}}
{{- end -}}

{{/*
Config input for kube_controllermanage
*/}}
{{- define "elasticagent.kubernetes.config.kube_controller.input" -}}
- id: kubernetes/metrics-kube-controllermanager
  type: kubernetes/metrics
  data_stream:
    namespace: {{ .Values.kubernetes.namespace }}
  use_output: {{ .Values.kubernetes.output }}
  streams:
  - id: kubernetes/metrics-kubernetes.controllermanager
    data_stream:
      type: metrics
      dataset: kubernetes.controllermanager
    metricsets:
      - controllermanager
{{- $vars := (include "elasticagent.kubernetes.config.kube_controller.default_vars" .) | fromYaml -}}
{{- mergeOverwrite $vars .Values.kubernetes.controller_manager.vars | toYaml | nindent 4 }}
{{- end -}}


{{/*
Defaults for kube_controller input streams
*/}}
{{- define "elasticagent.kubernetes.config.kube_controller.default_vars" -}}
hosts:
 - "https://0.0.0.0:10257"
period: "10s"
bearer_token_file: "var/run/secrets/kubernetes.io/serviceaccount/token"
ssl.verification_mode: "none"
condition: "${kubernetes.labels.component} == ''kube-controller-manager''"
{{- end -}}