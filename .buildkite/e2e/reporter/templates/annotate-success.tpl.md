{{- range $envName, $sortedTests := .TestsMap }}
{{- if gt (len $sortedTests.Passed) 0 }}

<p>
<details>
<summary>🐸 <code>{{ len $sortedTests.Passed }} tests</code> ~ {{ $envName }}</summary>

```
{{- range $test := $sortedTests.Passed }}
{{ $test.Name }} {{ $test.Duration }}
{{- end }}
```

</details>
</p>

{{- end }}
{{- end }}