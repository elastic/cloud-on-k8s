{{- define "packages" -}}
// Generated documentation. Please do not edit.
[id="{p}-api-reference"]
== API Reference
{{ with .packages }}
.Packages

{{- range . }}
* xref:{p}-{{ packageAnchorID . | safeIdentifier }}[{{ packageDisplayName . }}]
{{- end }}
{{- end }}

'''

{{ range .packages }}
[id="{p}-{{ packageAnchorID . | safeIdentifier }}"]
=== {{ packageDisplayName . }}
{{- with (index .GoPackages 0 ) }}
{{- with .DocComments }}
{{ safe (renderComments .) }}
{{- end }}
{{- end }}

.Resource Types
--
{{- range (visibleTypes (sortedTypes .Types)) }}
{{- if isExportedType . }}
- {{ template "link_template" . }}
{{- end }}
{{- end }}
--

{{- range (visibleTypes (sortedTypes .Types)) }}

{{ template "type" .  }}
{{- end }}

{{- end }}
{{- end }}
