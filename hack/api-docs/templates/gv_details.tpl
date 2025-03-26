{{- define "gvDetails" -}}
{{- $gv := . -}}

% TODO add function to crd-ref-docs return anchor used in links docs-v3 does not seem to produce valid markdown anchors
## {{ $gv.GroupVersionString }} [#{{ markdownGroupVersionID $gv | replace "-" "" }}]

{{ $gv.Doc }}

{{- if $gv.Kinds  }}
### Resource Types
{{- range $gv.SortedKinds }}
- {{ $gv.TypeForKind . | markdownRenderTypeLink }}
{{- end }}
{{ end }}

{{ range $gv.SortedTypes }}
{{ template "type" . }}
{{ end }}

{{- end -}}
