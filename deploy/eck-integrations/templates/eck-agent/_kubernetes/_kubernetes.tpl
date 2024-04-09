{{- define "elasticagent.kubernetes.init" -}}
{{- if eq $.Values.kubernetes.enabled true -}}
{{- include "elasticagent.kubernetes.config.kube_apiserver.init" $ -}}
{{- include "elasticagent.kubernetes.config.state.containers.init" $ -}}
{{- include "elasticagent.kubernetes.config.state.cronjobs.init" $ -}}
{{- include "elasticagent.kubernetes.config.state.daemonsets.init" $ -}}
{{- include "elasticagent.kubernetes.config.state.deployments.init" $ -}}
{{- include "elasticagent.kubernetes.config.state.jobs.init" $ -}}
{{- include "elasticagent.kubernetes.config.state.namespaces.init" $ -}}
{{- include "elasticagent.kubernetes.config.state.nodes.init" $ -}}
{{- include "elasticagent.kubernetes.config.state.persistentvolumeclaims.init" $ -}}
{{- include "elasticagent.kubernetes.config.state.persistentvolumes.init" $ -}}
{{- include "elasticagent.kubernetes.config.state.pods.init" $ -}}
{{- include "elasticagent.kubernetes.config.state.replicasets.init" $ -}}
{{- include "elasticagent.kubernetes.config.state.resourcequotas.init" $ -}}
{{- include "elasticagent.kubernetes.config.state.services.init" $ -}}
{{- include "elasticagent.kubernetes.config.state.statefulsets.init" $ -}}
{{- include "elasticagent.kubernetes.config.state.storageclasses.init" $ -}}
{{- include "elasticagent.kubernetes.config.kube_controller.init" $ -}}
{{- include "elasticagent.kubernetes.config.hints.init" $ -}}
{{- include "elasticagent.kubernetes.config.audit_logs.init" $ -}}
{{- include "elasticagent.kubernetes.config.container_logs.init" $ -}}
{{- include "elasticagent.kubernetes.config.kubelet.containers.init" $ -}}
{{- include "elasticagent.kubernetes.config.kubelet.nodes.init" $ -}}
{{- include "elasticagent.kubernetes.config.kubelet.pods.init" $ -}}
{{- include "elasticagent.kubernetes.config.kubelet.system.init" $ -}}
{{- include "elasticagent.kubernetes.config.kubelet.volumes.init" $ -}}
{{- include "elasticagent.kubernetes.config.kube_proxy.init" $ -}}
{{- include "elasticagent.kubernetes.config.kube_scheduler.init" $ -}}
{{- end -}}
{{- end -}}