{{- define "elasticagent.kubernetes.config.state.replicasets.init" -}}
{{- if eq ((.Values.kubernetes.state).enabled) false -}}
{{- $_ := set $.Values.kubernetes.replicasets.state "enabled" false -}}
{{- else -}}
{{- if eq $.Values.kubernetes.replicasets.state.enabled true -}}
{{- $preset := $.Values.eck_agent.presets.clusterWide -}}
{{- $inputVal := (include "elasticagent.kubernetes.config.state.replicasets.input" $ | fromYamlArray) -}}
{{- include "elasticagent.preset.mutate.inputs" (list $ $preset $inputVal) -}}
{{- include "elasticagent.preset.applyOnce" (list $ $preset "elasticagent.kubernetes.pernode.preset") -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "elasticagent.kubernetes.config.state.replicasets.input" -}}
- id: kubernetes/metrics-kubernetes.state_replicaset
  type: kubernetes/metrics
  data_stream:
    namespace: {{ $.Values.kubernetes.namespace }}
  use_output: {{ $.Values.kubernetes.output }}
  streams:
  - id: kubernetes/metrics-kubernetes.state_replicaset
    data_stream:
      type: metrics
      dataset: kubernetes.state_replicaset
    metricsets:
      - state_replicaset
{{- $defaults := (include "elasticagent.kubernetes.config.state.replicasets.default_vars" $ ) | fromYaml -}}
{{- mergeOverwrite $defaults .Values.kubernetes.replicasets.state.vars | toYaml | nindent 4 -}}
{{- end -}}

{{- define "elasticagent.kubernetes.config.state.replicasets.default_vars" -}}
add_metadata: true
hosts:
{{- if eq $.Values.kubernetes.state.deployKSM true }}
  - 'localhost:8080'
{{- else -}}
  - 'kube-state-metrics:8080'
{{- end }}
period: 10s
bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
{{- end -}}