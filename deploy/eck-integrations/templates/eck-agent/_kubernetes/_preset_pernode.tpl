{{- define "elasticagent.kubernetes.pernode.preset" -}}
{{- include "elasticagent.preset.mutate.rules" (list $ $.Values.agent.presets.perNode "elasticagent.kubernetes.pernode.preset.rules") -}}
{{- include "elasticagent.preset.mutate.volumemounts" (list $ $.Values.agent.presets.perNode "elasticagent.kubernetes.pernode.preset.volumemounts") -}}
{{- include "elasticagent.preset.mutate.volumes" (list $ $.Values.agent.presets.perNode "elasticagent.kubernetes.pernode.preset.volumes") -}}
{{- include "elasticagent.preset.mutate.elasticsearchrefs.byname" (list $ $.Values.agent.presets.perNode $.Values.kubernetes.output)}}
{{- if eq $.Values.kubernetes.hints.enabled true -}}
{{- include "elasticagent.preset.mutate.initcontainers" (list $ $.Values.agent.presets.perNode "elasticagent.kubernetes.pernode.preset.initcontainers") -}}
{{- include "elasticagent.preset.mutate.providers.kubernetes.hints" (list $ $.Values.agent.presets.perNode "elasticagent.kubernetes.pernode.preset.providers.kubernetes.hints") -}}
{{- end -}}
{{- if or (eq $.Values.kubernetes.scheduler.enabled true) (eq $.Values.kubernetes.controller_manager.enabled true) -}}
{{- include "elasticagent.preset.mutate.tolerations" (list $ $.Values.agent.presets.perNode "elasticagent.kubernetes.pernode.preset.tolerations") -}}
{{- end -}}
{{- end -}}

{{- define "elasticagent.kubernetes.pernode.preset.rules" -}}
rules:
- apiGroups: [""] # "" indicates the core API group
  resources:
  - namespaces
  - pods
  - persistentvolumes
  - persistentvolumeclaims
  - persistentvolumeclaims/status
  - nodes
  - nodes/metrics
  - configmaps
  - nodes/proxy
  - nodes/stats
  - services
  - events
  verbs:
  - get
  - watch
  - list
- apiGroups:
  - storage.k8s.io
  resources:
  - storageclasses
  verbs:
  - get
  - watch
  - list
- nonResourceURLs:
  - /metrics
  verbs:
  - get
  - watch
  - list
- apiGroups: ["coordination.k8s.io"]
  resources:
  - leases
  verbs:
  - get
  - create
  - update
- nonResourceURLs:
  - /healthz
  - /healthz/*
  - /livez
  - /livez/*
  - /metrics
  - /metrics/slis
  - /readyz
  - /readyz/*
  verbs:
  - get
- apiGroups: ["apps"]
  resources:
  - replicasets
  - deployments
  - daemonsets
  - statefulsets
  verbs:
  - get
  - list
  - watch
- apiGroups: ["batch"]
  resources:
  - jobs
  - cronjobs
  verbs:
  - get
  - list
  - watch
{{- end -}}

{{- define "elasticagent.kubernetes.pernode.preset.volumemounts" -}}
extraVolumeMounts:
- name: proc
  mountPath: /hostfs/proc
  readOnly: true
- name: cgroup
  mountPath: /hostfs/sys/fs/cgroup
  readOnly: true
- name: varlibdockercontainers
  mountPath: /var/lib/docker/containers
  readOnly: true
- name: varlog
  mountPath: /var/log
  readOnly: true
- name: etc-full
  mountPath: /hostfs/etc
  readOnly: true
- name: var-lib
  mountPath: /hostfs/var/lib
  readOnly: true
{{- if eq $.Values.kubernetes.hints.enabled true }}
- name: external-inputs
  mountPath: /usr/share/elastic-agent/state/inputs.d
{{- end }}
{{- end -}}

{{- define "elasticagent.kubernetes.pernode.preset.volumes" -}}
extraVolumes:
- name: proc
  hostPath:
    path: /proc
- name: cgroup
  hostPath:
    path: /sys/fs/cgroup
- name: varlibdockercontainers
  hostPath:
    path: /var/lib/docker/containers
- name: varlog
  hostPath:
    path: /var/log
- name: etc-full
  hostPath:
    path: /etc
- name: var-lib
  hostPath:
    path: /var/lib
{{- if eq $.Values.kubernetes.hints.enabled true }}
- name: external-inputs
  emptyDir: {}
{{- end }}
{{- end -}}

{{- define "elasticagent.kubernetes.pernode.preset.initcontainers" -}}
initContainers:
- name: k8s-templates-downloader
  image: busybox:1.36
  command: [ 'sh' ]
  args:
    - -c
    - >-
      mkdir -p /etc/elastic-agent/inputs.d &&
      wget -O - https://github.com/elastic/elastic-agent/archive/v{{$.Values.agent.version}}.tar.gz | tar xz -C /etc/elastic-agent/inputs.d --strip=5 "elastic-agent-{{$.Values.agent.version}}/deploy/kubernetes/elastic-agent-standalone/templates.d"
  volumeMounts:
    - name: external-inputs
      mountPath: /etc/elastic-agent/inputs.d
{{- end -}}

{{- define "elasticagent.kubernetes.pernode.preset.providers.kubernetes.hints" -}}
providers:
  kubernetes:
    hints:
      enabled: true
{{- end -}}

{{- define "elasticagent.kubernetes.pernode.preset.tolerations" -}}
tolerations:
  - key: node-role.kubernetes.io/control-plane
    effect: NoSchedule
  - key: node-role.kubernetes.io/master
    effect: NoSchedule
{{- end -}}