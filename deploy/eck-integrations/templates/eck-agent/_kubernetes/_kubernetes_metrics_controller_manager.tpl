{{/*
Config input for kube_controllermanage
*/}}
{{- define "kubernetes.config.kube_controller.input" -}}
{{- if default .control_plane.controller_manager.enabled false -}}
- id: kubernetes/metrics-kube-controllermanage
  revision: 1
  name: kubernetes
  type: kubernetes/metrics
  data_stream:
    namespace: {{ .namespace }}
  use_output: default
  package_policy_id: {{.integrationID}}
  streams:
    - id: kubernetes/metrics-kubernetes.controllermanager
      data_stream:
        type: metrics
        dataset: kubernetes.controllermanager
      metricsets:
        - controllermanager
{{- include "kubernetes.config.kube_controller.defaults" .control_plane.controller_manager | nindent 4 }} 
  meta:
    package:
      name: kubernetes
      version: {{ .version }}
{{- end -}}
{{- end -}}


{{/*
Defaults for kube_controller input streams
*/}}
{{- define "kubernetes.config.kube_controller.defaults" -}}
hosts:
{{- range dig "vars" "hosts" (list "https://0.0.0.0:10257") . }}
- {{. | quote}}
{{- end }}
period: {{ dig "vars" "period" "10s" . }}
bearer_token_file: {{ dig "vars" "bearer_token_file" "/var/run/secrets/kubernetes.io/serviceaccount/token" .}}
ssl.verification_mode: {{ dig "vars" "ssl.verification_mode" "none" . }}
condition: {{ dig "vars" "condition" "${kubernetes.labels.component} == ''kube-controller-manager''" . | quote }}
{{- end -}}