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
{{- include "eck-operator.selectorLabels" . }}
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
{{- if .Values.global.manifestGen }}
control-plane: elastic-operator
{{- else }}
app.kubernetes.io/name: {{ include "eck-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
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
Determine effective Kubernetes version
*/}}
{{- define "eck-operator.effectiveKubeVersion" -}}
{{- if .Values.global.manifestGen -}}
{{- semver .Values.global.kubeVersion -}}
{{- else -}}
{{- .Capabilities.KubeVersion.Version -}}
{{- end -}}
{{- end -}}

{{/*
Determine the name for the webhook 
*/}}
{{- define "eck-operator.webhookName" -}}
{{- if .Values.global.manifestGen -}}
elastic-webhook.k8s.elastic.co
{{- else -}}
{{- $name := include "eck-operator.name" . -}}
{{ printf "%s.%s.k8s.elastic.co" $name .Release.Namespace }}
{{- end -}}
{{- end -}}

{{/*
Determine the name for the webhook secret 
*/}}
{{- define "eck-operator.webhookSecretName" -}}
{{- if .Values.global.manifestGen -}}
elastic-webhook-server-cert
{{- else -}}
{{- $name := include "eck-operator.name" . -}}
{{ printf "%s-webhook-cert" $name | trunc 63 }}
{{- end -}}
{{- end -}}

{{/*
Determine the name for the webhook service 
*/}}
{{- define "eck-operator.webhookServiceName" -}}
{{- if .Values.global.manifestGen -}}
elastic-webhook-server
{{- else -}}
{{- $name := include "eck-operator.name" . -}}
{{ printf "%s-webhook" $name | trunc 63 }}
{{- end -}}
{{- end -}}

{{/*
RBAC permissions
NOTE - any changes made to RBAC permissions below require
updating docs/operating-eck/eck-permissions.asciidoc file.
*/}}
{{- define "eck-operator.rbacRules" -}}
- apiGroups:
  - "authorization.k8s.io"
  resources:
  - subjectaccessreviews
  verbs:
  - create
- apiGroups:
  - coordination.k8s.io
  resources:
  - leases
  verbs:
  - create
- apiGroups:
  - coordination.k8s.io
  resources:
  - leases
  resourceNames:
  - elastic-operator-leader
  verbs:
  - get
  - watch
  - update
- apiGroups:
  - ""
  resources:
  - endpoints
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - pods
  - events
  - persistentvolumeclaims
  - secrets
  - services
  - configmaps
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
  - elasticsearches/finalizers # needed for ownerReferences with blockOwnerDeletion on OCP
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
- apiGroups:
  - kibana.k8s.elastic.co
  resources:
  - kibanas
  - kibanas/status
  - kibanas/finalizers # needed for ownerReferences with blockOwnerDeletion on OCP
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
- apiGroups:
  - apm.k8s.elastic.co
  resources:
  - apmservers
  - apmservers/status
  - apmservers/finalizers # needed for ownerReferences with blockOwnerDeletion on OCP
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
- apiGroups:
  - enterprisesearch.k8s.elastic.co
  resources:
  - enterprisesearches
  - enterprisesearches/status
  - enterprisesearches/finalizers # needed for ownerReferences with blockOwnerDeletion on OCP
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
- apiGroups:
  - beat.k8s.elastic.co
  resources:
  - beats
  - beats/status
  - beats/finalizers # needed for ownerReferences with blockOwnerDeletion on OCP
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
- apiGroups:
  - agent.k8s.elastic.co
  resources:
  - agents
  - agents/status
  - agents/finalizers # needed for ownerReferences with blockOwnerDeletion on OCP
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
- apiGroups:
  - maps.k8s.elastic.co
  resources:
  - elasticmapsservers
  - elasticmapsservers/status
  - elasticmapsservers/finalizers # needed for ownerReferences with blockOwnerDeletion on OCP
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
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
- apiGroups:
  - admissionregistration.k8s.io
  resources:
  - validatingwebhookconfigurations
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
RBAC permissions to read node labels
*/}}
{{- define "eck-operator.readNodeLabelsRbacRule" -}}
- apiGroups:
  - ""
  resources:
  - nodes
  verbs:
  - get
  - list
  - watch
{{- end -}}
