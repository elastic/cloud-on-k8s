{{- define "elasticagent.kubernetes.clusterwide.preset" -}}
{{- include "elasticagent.preset.mutate.rules" (list $ $.Values.eck_agent.presets.clusterWide "elasticagent.kubernetes.clusterwide.preset.rules") -}}
{{- include "elasticagent.preset.mutate.elasticsearchrefs.byname" (list $ $.Values.eck_agent.presets.clusterWide $.Values.kubernetes.output)}}
{{- end -}}

{{- define "elasticagent.kubernetes.clusterwide.preset.rules" -}}
rules:
# minimum cluster role ruleset required by agent
- apiGroups: [ "" ]
  resources:
    - nodes
    - namespaces
    - pods
  verbs:
    - get
    - watch
    - list
- nonResourceURLs:
    - /metrics
  verbs:
    - get
    - watch
    - list
- apiGroups:
    - coordination.k8s.io
  resources:
    - leases
  verbs:
    - create
    - update
    - get
    - list
    - watch
- apiGroups: [ "apps" ]
  resources:
    - replicasets
  verbs:
    - get
    - list
    - watch
- apiGroups: [ "batch" ]
  resources:
    - jobs
  verbs:
    - get
    - list
    - watch
{{- end -}}