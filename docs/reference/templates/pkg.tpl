{{- define "packages" -}}
// Generated documentation. Please do not edit.
[id="{p}-reference"]
== Reference
{{ with .packages }}
.Packages
{{- range . }}
- xref:{{ packageAnchorID . | safeIdentifier }}[{{ packageDisplayName . }}]
{{- end }}
{{- end }}

{{ range .packages }}
[id="{{ packageAnchorID . | safeIdentifier }}"]
=== {{ packageDisplayName . }}
{{- with (index .GoPackages 0 ) }}
{{- with .DocComments }}
{{ safe (renderComments .) }}
{{- end }}
{{- end }}

.Resource Types
{{- range (visibleTypes (sortedTypes .Types)) }}
{{- if isExportedType . }}
link:{{ linkForType . }}[{{ typeDisplayName . }}]
{{- end }}
{{- end }}

{{ range (visibleTypes (sortedTypes .Types)) }}
{{ template "type" .  }}
{{- end }}
{{- end }}
{{- end }}
