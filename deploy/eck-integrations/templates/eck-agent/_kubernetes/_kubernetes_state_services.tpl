{{- define "elasticagent.kubernetes.config.state.services.init" -}}
{{- if eq ((.Values.kubernetes.state).enabled) false -}}
{{- $_ := set $.Values.kubernetes.services.state "enabled" false -}}
{{- else -}}
{{- if eq $.Values.kubernetes.services.state.enabled true -}}
{{- $preset := $.Values.agent.presets.clusterWide -}}
{{- if eq $.Values.kubernetes.state.deployKSM true -}}
{{- $preset = $.Values.agent.presets.ksmSharded -}}
{{- include "elasticagent.preset.applyOnce" (list $ $preset "elasticagent.kubernetes.ksmsharded.preset") -}}
{{- end -}}
{{- $inputVal := (include "elasticagent.kubernetes.config.state.services.input" $ | fromYamlArray) -}}
{{- include "elasticagent.preset.mutate.inputs" (list $ $preset $inputVal) -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "elasticagent.kubernetes.config.state.services.input" -}}
- id: kubernetes/metrics-kubernetes.state_service
  type: kubernetes/metrics
  data_stream:
    namespace: {{ $.Values.kubernetes.namespace }}
  use_output: {{ $.Values.kubernetes.output }}
  streams:
  - id: kubernetes/metrics-kubernetes.state_service
    data_stream:
      type: metrics
      dataset: kubernetes.state_service
    use_output: {{ .Values.kubernetes.output }}
    metricsets:
      - state_service
{{- $defaults := (include "elasticagent.kubernetes.config.state.services.default_vars" $ ) | fromYaml -}}
{{- mergeOverwrite $defaults .Values.kubernetes.services.state.vars | toYaml | nindent 4 }}
{{- end -}}

{{- define "elasticagent.kubernetes.config.state.services.default_vars" -}}
add_metadata: true
hosts:
{{- if eq $.Values.kubernetes.state.deployKSM true }}
  - 'localhost:8080'
{{- else }}
  - {{ $.Values.kubernetes.state.host }}
{{- end }}
period: 10s
{{- if eq $.Values.kubernetes.state.deployKSM false }}
condition: '${kubernetes_leaderelection.leader} == true'
{{- end }}
bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
{{- end -}}