{{- define "elasticagent.kubernetes.config.hints.init" -}}
{{- if eq $.Values.kubernetes.hints.enabled true -}}
{{- $preset := $.Values.eck_agent.presets.perNode -}}
{{- include "elasticagent.preset.applyOnce" (list $ $preset "elasticagent.kubernetes.pernode.preset") -}}
{{- end -}}
{{- end -}}
