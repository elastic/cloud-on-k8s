{{- define "elasticagent.kubernetes.init" -}}
{{- if eq $.Values.kubernetes.enabled true -}}
{{- if not (hasKey $.Values.elasticsearchRefs $.Values.kubernetes.output) -}}
{{- fail (printf "output \"%s\" of kubernetes integration is not defined" $.Values.kubernetes.output)}}
{{- end -}}
{{- include "elasticagent.kubernetes.config.kube_apiserver.init" $ -}}
{{- include "elasticagent.kubernetes.config.kube_state.init" $ -}}
{{- include "elasticagent.kubernetes.config.kube_controller.init" $ -}}
{{- include "elasticagent.kubernetes.config.hints.init" $ -}}
{{- include "elasticagent.kubernetes.config.audit_logs.init" $ -}}
{{- include "elasticagent.kubernetes.config.container_logs.init" $ -}}
{{- include "elasticagent.kubernetes.config.kubelet.init" $ -}}
{{- include "elasticagent.kubernetes.config.kube_proxy.init" $ -}}
{{- include "elasticagent.kubernetes.config.kube_scheduler.init" $ -}}
{{- end -}}
{{- end -}}