{{- $kubeVersion := (include "eck-operator-crds.effectiveKubeVersion" .) -}}
{{- $kubeVersionSupported := semverCompare ">=1.16.0-0" $kubeVersion -}}
{{- if $kubeVersionSupported -}}
