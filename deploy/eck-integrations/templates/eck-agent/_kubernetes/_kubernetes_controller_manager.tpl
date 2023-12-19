{{- define "agent.kubernetes.config.kube_controller.enabled" -}}
enabled: {{ .Values.kubernetes.controller_manager.enabled }}
{{- end -}}

{{/*
Config input for kube_controllermanage
*/}}
{{- define "agent.kubernetes.config.kube_controller.input" -}}
{{- $vars := (include "agent.kubernetes.config.kube_controller.default_vars" .) | fromYaml -}}
- id: kubernetes/metrics-kube-controllermanager
  type: kubernetes/metrics
  data_stream:
    namespace: {{ .Values.kubernetes.namespace }}
  use_output: default
  streams:
    - id: kubernetes/metrics-kubernetes.controllermanager
      data_stream:
        type: metrics
        dataset: kubernetes.controllermanager
      metricsets:
        - controllermanager
{{- mergeOverwrite $vars .Values.kubernetes.controller_manager.vars | toYaml | nindent 4 -}}
  meta:
    package:
      name: kubernetes
      version: {{ .Values.kubernetes.version }}
{{- end -}}


{{/*
Defaults for kube_controller input streams
*/}}
{{- define "agent.kubernetes.config.kube_controller.default_vars" -}}
hosts:
 - "https://0.0.0.0:10257"
period: "10s"
bearer_token_file: "var/run/secrets/kubernetes.io/serviceaccount/token"
ssl.verification_mode: "none"
condition: "${kubernetes.labels.component} == ''kube-controller-manager''"
{{- end -}}