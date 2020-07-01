{{/* vim: set filetype=mustache: */}}

{{/*
RBAC permissions
*/}}
{{- define "rbac.rules" -}}
- apiGroups:
  - "authorization.k8s.io"
  resources:
  - subjectaccessreviews
  verbs:
  - create
- apiGroups:
  - ""
  resources:
  - pods
  - endpoints
  - events
  - persistentvolumeclaims
  - secrets
  - services
  - configmaps
  - serviceaccounts
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - apps
  resources:
  - deployments
  - statefulsets
  - daemonsets
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - policy
  resources:
  - poddisruptionbudgets
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - elasticsearch.k8s.elastic.co
  resources:
  - elasticsearches
  - elasticsearches/status
  - elasticsearches/finalizers
  - enterpriselicenses
  - enterpriselicenses/status
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - kibana.k8s.elastic.co
  resources:
  - kibanas
  - kibanas/status
  - kibanas/finalizers
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - apm.k8s.elastic.co
  resources:
  - apmservers
  - apmservers/status
  - apmservers/finalizers
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - enterprisesearch.k8s.elastic.co
  resources:
  - enterprisesearches
  - enterprisesearches/status
  - enterprisesearches/finalizers
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - admissionregistration.k8s.io
  resources:
  - mutatingwebhookconfigurations
  - validatingwebhookconfigurations
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - beat.k8s.elastic.co
  resources:
  - beats
  - beats/status
  - beats/finalizers
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
{{- end -}}

