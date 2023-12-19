{{/*
Expand the name of the chart.
*/}}
{{- define "elasticagent.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "elasticagent.fullname" -}}
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
*/}}
{{- define "elasticagent.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "elasticagent.labels" -}}
helm.sh/chart: {{ include "elasticagent.chart" . }}
{{ include "elasticagent.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- if .Values.labels }}
{{ toYaml .Values.labels }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "elasticagent.selectorLabels" -}}
app.kubernetes.io/name: {{ include "elasticagent.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "elasticagent.elasticOutput" -}}
{{- $host := trim .Values.elasticsearch.host -}}
{{- $_ := required "Specifying an elastic search host is required!" $host }}
default:
  type: elasticsearch
  hosts:
    - {{ $host | quote }}
{{- $found := "" -}}
{{- $user := trim .Values.elasticsearch.user -}}
{{- $pass := trim .Values.elasticsearch.pass -}}
{{- if and $user $pass -}}
{{- $found = "true" }}
  username: {{ $user | quote  }}
  password: {{ $pass | quote  }}
{{- end -}}
{{ $apiKey := trim .Values.elasticsearch.apiKey -}}
{{- if and (empty $found) $apiKey -}}
{{- $found = "true" }}
  api_key: {{ .Values.elasticsearch.apiKey | quote  }}
{{- end -}}
{{- $_ := required "Specifying either user,pass or api_key for elastic search is required!" $found -}}
{{- end }}

{{- define "elasticagent.deployment" -}}
{{- if not (hasKey $.Values.eck_agent "deployment") -}}
{{- $_ := set .Values.eck_agent "deployment" dict -}}
{{- $configsToCheck := (list) -}}
{{- if .Values.kubernetes.enabled -}}
{{- $configsToCheck = append $configsToCheck "agent.kubernetes.config.kube_state" -}}
{{- $configsToCheck = append $configsToCheck "agent.kubernetes.config.hints" -}}
{{- $configsToCheck = append $configsToCheck "agent.kubernetes.config.kube_apiserver" -}}
{{- end -}}
{{- $enabledConfigs := (list) -}}
{{- range $configTmplName := $configsToCheck -}}
{{- $tplName := print $configTmplName ".enabled" -}}
{{- $inputConfig := (include $tplName $ | fromYaml) -}}
{{- if $inputConfig.enabled -}}
{{- $enabledConfigs = append $enabledConfigs $configTmplName }}
{{- end -}}
{{- end -}}
{{- if empty $enabledConfigs -}}
{{- $_ := set .Values.eck_agent.deployment "enabled" false -}}
{{- $_ := set .Values.eck_agent.deployment "enabled_configs" list -}}
{{- else -}}
{{- $_ := set .Values.eck_agent.deployment "enabled" true -}}
{{- $_ := set .Values.eck_agent.deployment "enabled_configs" $enabledConfigs -}}
{{- end -}}
{{- end -}}
{{- end -}}


{{- define "elasticagent.daemonset" -}}
{{- if not (hasKey $.Values.eck_agent "daemonset") -}}
{{- $_ := set .Values.eck_agent "daemonset" dict -}}
{{- $configsToCheck := (list) -}}
{{- if .Values.kubernetes.enabled -}}
{{- $configsToCheck = append $configsToCheck "agent.kubernetes.config.kube_controller" -}}
{{- $configsToCheck = append $configsToCheck "agent.kubernetes.config.kube_scheduler" -}}
{{- $configsToCheck = append $configsToCheck "agent.kubernetes.config.audit_logs" -}}
{{- $configsToCheck = append $configsToCheck "agent.kubernetes.config.container_logs" -}}
{{- $configsToCheck = append $configsToCheck "agent.kubernetes.config.kubelet" -}}
{{- $configsToCheck = append $configsToCheck "agent.kubernetes.config.kube_proxy" -}}
{{- end -}}
{{- if .Values.cloudDefend.enabled -}}
{{- $configsToCheck = append $configsToCheck "agent.cloud_defend.config" -}}
{{- end -}}
{{- $enabledConfigs := (list) -}}
{{- range $configTmplName := $configsToCheck -}}
{{- $tplName := print $configTmplName ".enabled" -}}
{{- $inputConfig := (include $tplName $ | fromYaml) -}}
{{- if $inputConfig.enabled -}}
{{- $enabledConfigs = append $enabledConfigs $configTmplName }}
{{- end -}}
{{- end -}}
{{- if empty $enabledConfigs -}}
{{- $_ := set .Values.eck_agent.daemonset "enabled" false -}}
{{- $_ := set .Values.eck_agent.daemonset "enabled_configs" list -}}
{{- else -}}
{{- $_ := set .Values.eck_agent.daemonset "enabled" true -}}
{{- $_ := set .Values.eck_agent.daemonset "enabled_configs" $enabledConfigs -}}
{{- end -}}
{{- end -}}
{{- end -}}
