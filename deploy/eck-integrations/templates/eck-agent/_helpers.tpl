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
{{- end }}

{{/*
Selector labels
*/}}
{{- define "elasticagent.selectorLabels" -}}
app.kubernetes.io/name: {{ include "elasticagent.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "elasticagent.preset.applyOnce" -}}
{{- $ := index . 0 -}}
{{- $preset := index . 1 -}}
{{- $templateName := index . 2 -}}
{{- if not (hasKey $preset "_appliedMutationTemplates") -}}
{{- $_ := set $preset "_appliedMutationTemplates" dict }}
{{- end -}}
{{- $appliedMutationTemplates := get $preset "_appliedMutationTemplates" -}}
{{- if not (hasKey $appliedMutationTemplates $templateName) -}}
{{- include $templateName $ -}}
{{- $_ := set $appliedMutationTemplates $templateName dict}}
{{- end -}}
{{- end -}}

{{- define "elasticagent.preset.mutate.inputs" -}}
{{- $ := index . 0 -}}
{{- $preset := index . 1 -}}
{{- $inputVal := index . 2 -}}
{{- $presetInputs := dig "_inputs" (list) $preset -}}
{{- $presetInputs = uniq (concat $presetInputs $inputVal) -}}
{{- $_ := set $preset "_inputs" $presetInputs -}}
{{- end -}}

{{- define "elasticagent.preset.mutate.securityContext.capabilities.add" -}}
{{- $ := index . 0 -}}
{{- $preset := index . 1 -}}
{{- $templateName := index . 2 -}}
{{- if not (hasKey $preset "securityContext") -}}
{{- $_ := set $preset "securityContext" dict }}
{{- end -}}
{{- $presetSecurityContext := get $preset "securityContext" }}
{{- if not (hasKey $presetSecurityContext "capabilities") -}}
{{- $_ := set $presetSecurityContext "capabilities" dict }}
{{- end -}}
{{- $presetSecurityContextCapabilities := get $presetSecurityContext "capabilities" }}
{{- if not (hasKey $presetSecurityContextCapabilities "add") -}}
{{- $_ := set $presetSecurityContextCapabilities "add" list }}
{{- end -}}
{{- $presetSecurityContextCapabilitiesAdd := get $presetSecurityContextCapabilities "add" }}
{{- $capabilitiesAddToAdd := dig "securityContext" "capabilities" "add" (list) (include $templateName $ | fromYaml) -}}
{{- $presetSecurityContextCapabilitiesAdd = uniq (concat $presetSecurityContextCapabilitiesAdd $capabilitiesAddToAdd) -}}
{{- $_ := set $presetSecurityContextCapabilities "add" $presetSecurityContextCapabilitiesAdd -}}
{{- end -}}

{{- define "elasticagent.preset.mutate.providers.kubernetes.hints" -}}
{{- $ := index . 0 -}}
{{- $preset := index . 1 -}}
{{- $templateName := index . 2 -}}
{{- if not (hasKey $preset "providers") -}}
{{- $_ := set $preset "providers" dict }}
{{- end -}}
{{- $presetProviders := get $preset "providers" }}
{{- if not (hasKey $presetProviders "kubernetes") -}}
{{- $_ := set $presetProviders "kubernetes" dict }}
{{- end -}}
{{- $presetProvidersKubernetes := get $presetProviders "kubernetes" }}
{{- if not (hasKey $presetProvidersKubernetes "hints") -}}
{{- $_ := set $presetProvidersKubernetes "hints" dict }}
{{- end -}}
{{- $presetProvidersKubernetesHints := get $presetProvidersKubernetes "hints" }}
{{- $presetProvidersKubernetesHintsToAdd := dig "providers" "kubernetes" "hints" (dict) (include $templateName $ | fromYaml) -}}
{{- $presetProvidersKubernetesHints = merge $presetProvidersKubernetesHintsToAdd $presetProvidersKubernetesHints -}}
{{- $_ := set $presetProvidersKubernetes "hints" $presetProvidersKubernetesHints -}}
{{- end -}}

{{- define "elasticagent.preset.mutate.rules" -}}
{{- $ := index . 0 -}}
{{- $preset := index . 1 -}}
{{- $templateName := index . 2 -}}
{{- $presetRules := dig "rules" (list) $preset -}}
{{- $rulesToAdd := get (include $templateName $ | fromYaml) "rules" -}}
{{- $presetRules = uniq (concat $presetRules $rulesToAdd) -}}
{{- $_ := set $preset "rules" $presetRules -}}
{{- end -}}

{{- define "elasticagent.preset.mutate.containers" -}}
{{- $ := index . 0 -}}
{{- $preset := index . 1 -}}
{{- $templateName := index . 2 -}}
{{- $presetContainers := dig "extraContainers" (list) $preset -}}
{{- $containersToAdd := get (include $templateName $ | fromYaml) "extraContainers"}}
{{- $presetContainers = uniq (concat $presetContainers $containersToAdd) -}}
{{- $_ := set $preset "extraContainers" $presetContainers -}}
{{- end -}}

{{- define "elasticagent.preset.mutate.tolerations" -}}
{{- $ := index . 0 -}}
{{- $preset := index . 1 -}}
{{- $templateName := index . 2 -}}
{{- $tolerationsToAdd := dig "tolerations" (list) (include $templateName $ | fromYaml) }}
{{- if $tolerationsToAdd -}}
{{- $presetTolerations := dig "tolerations" (list) $preset -}}
{{- $presetTolerations = uniq (concat $presetTolerations $tolerationsToAdd) -}}
{{- $_ := set $preset "tolerations" $tolerationsToAdd -}}
{{- end -}}
{{- end -}}

{{- define "elasticagent.preset.mutate.initcontainers" -}}
{{- $ := index . 0 -}}
{{- $preset := index . 1 -}}
{{- $templateName := index . 2 -}}
{{- $presetInitContainers := dig "initContainers" (list) $preset -}}
{{- $initContainersToAdd := get (include $templateName $ | fromYaml) "initContainers"}}
{{- $presetInitContainers = uniq (concat $presetInitContainers $initContainersToAdd) -}}
{{- $_ := set $preset "initContainers" $presetInitContainers -}}
{{- end -}}

{{- define "elasticagent.preset.mutate.volumes" -}}
{{- $ := index . 0 -}}
{{- $preset := index . 1 -}}
{{- $templateName := index . 2 -}}
{{- $presetVolumes := dig "extraVolumes" (list) $preset -}}
{{- $volumesToAdd := get (include $templateName $ | fromYaml) "extraVolumes"}}
{{- $presetVolumes = uniq (concat $presetVolumes $volumesToAdd) -}}
{{- $_ := set $preset "extraVolumes" $presetVolumes -}}
{{- end -}}

{{- define "elasticagent.preset.mutate.volumemounts" -}}
{{- $ := index . 0 -}}
{{- $preset := index . 1 -}}
{{- $templateName := index . 2 -}}
{{- $presetVolumeMounts := dig "extraVolumeMounts" (list) $preset -}}
{{- $volumeMountsToAdd := get (include $templateName $ | fromYaml) "extraVolumeMounts"}}
{{- $presetVolumeMounts = uniq (concat $presetVolumeMounts $volumeMountsToAdd) -}}
{{- $_ := set $preset "extraVolumeMounts" $presetVolumeMounts -}}
{{- end -}}

{{- define "elasticagent.preset.mutate.elasticsearchrefs.byname" -}}
{{- $ := index . 0 -}}
{{- $preset := index . 1 -}}
{{- $outputName := index . 2 -}}
{{- if not (hasKey $.Values.elasticsearchRefs $outputName) -}}
{{- fail (printf "output \"%s\" is not defined" $outputName) -}}
{{- end -}}
{{- $outputDict := get $.Values.elasticsearchRefs $outputName -}}
{{- $outputDict = deepCopy $outputDict -}}
{{- if and (not (hasKey $outputDict "secretName")) (not (hasKey $outputDict "name")) -}}
{{- fail (printf "either a \"secretName\" or \"name\" has to be specified for output \"%s\"" $outputName) -}}
{{- end -}}
{{- $_ := set $outputDict "outputName" $outputName -}}
{{- $presetElasticSearchRefs := dig "elasticSearchRefs" (list) $preset -}}
{{- $presetElasticSearchRefs = uniq (concat $presetElasticSearchRefs (list $outputDict)) -}}
{{- $_ := set $preset "elasticSearchRefs" $presetElasticSearchRefs -}}
{{- end -}}

{{- define "elasticagent.preset.init" -}}
{{- if not (hasKey $.Values.eck_agent "initialised") -}}
{{- include "elasticagent.kubernetes.init" $ -}}
{{- include "elasticagent.clouddefend.init" $ -}}
{{- range $presetName, $presetVal := $.Values.eck_agent.presets -}}
{{- $presetMode := dig "mode" ("") $presetVal -}}
{{- if not $presetMode -}}
{{- fail (printf "mode is missing from preset \"%s\"" $presetName)}}
{{- else if eq $presetMode "deployment" -}}
{{- else if eq $presetMode "statefulset" -}}
{{- else if eq $presetMode "daemonset" -}}
{{- else -}}
{{- fail (printf "invalid mode \"%s\" in preset \"%s\", must be one of deployment, statefulset, daemonset" $presetMode $presetName)}}
{{- end -}}
{{- $presetInputs := dig "_inputs" (list) $presetVal -}}
{{- if empty $presetInputs -}}
{{- $_ := unset $.Values.eck_agent.presets $presetName}}
{{- else -}}
{{- $monitoringOutput := dig "agent" "monitoring" "use_output" "" $presetVal -}}
{{- if $monitoringOutput -}}
{{- include "elasticagent.preset.mutate.elasticsearchrefs.byname" (list $ $presetVal $monitoringOutput) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- $_ := set $.Values.eck_agent "initialised" dict -}}
{{- end -}}
{{- end -}}
