{{/*
Expand the name of the chart.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
*/}}
{{- define "eck-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "eck-operator.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
*/}}
{{- define "eck-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "eck-operator.labels" -}}
{{ include "eck-operator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
helm.sh/chart: {{ include "eck-operator.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "eck-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "eck-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "eck-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "eck-operator.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
RBAC permissions
*/}}
{{- define "eck-operator.rbacRules" -}}
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

{{/*
RBAC permissions on non-namespaced resources
*/}}
{{- define "eck-operator.clusterWideRbacRules" -}}
- apiGroups:
  - storage.k8s.io
  resources:
  - storageclasses
  verbs:
  - get
  - list
  - watch
{{- end -}}
