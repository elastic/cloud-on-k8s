{{- define "elasticagent.kubernetes.clusterwide.preset" -}}
{{- include "elasticagent.preset.mutate.rules" (list $ $.Values.eck_agent.presets.clusterWide "elasticagent.kubernetes.clusterwide.preset.rules") -}}
{{- if eq $.Values.kubernetes.state.deployKSM true -}}
{{- include "elasticagent.preset.mutate.containers" (list $ $.Values.eck_agent.presets.clusterWide "elasticagent.kubernetes.clusterwide.ksm.containers") -}}
{{- end -}}
{{- end -}}

{{- define "elasticagent.kubernetes.clusterwide.preset.rules" -}}
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
      - nodes/proxy
      - nodes/stats
      - services
      - events
      - configmaps
      - secrets
      - nodes
      - pods
      - services
      - serviceaccounts
      - resourcequotas
      - replicationcontrollers
      - limitranges
      - endpoints
    verbs:
      - get
      - watch
      - list
  - apiGroups:
      - autoscaling
    resources:
      - horizontalpodautoscalers
    verbs:
      - get
      - list
      - watch
  - apiGroups:
      - authentication.k8s.io
    resources:
      - tokenreviews
    verbs:
      - create
  - apiGroups:
      - authorization.k8s.io
    resources:
      - subjectaccessreviews
    verbs:
      - create
  - apiGroups:
      - policy
    resources:
      - poddisruptionbudgets
    verbs:
      - get
      - list
      - watch
  - apiGroups:
      - certificates.k8s.io
    resources:
      - certificatesigningrequests
    verbs:
      - get
      - list
      - watch
  - apiGroups:
      - discovery.k8s.io
    resources:
      - endpointslices
    verbs:
      - list
      - watch
  - apiGroups:
      - storage.k8s.io
    resources:
      - storageclasses
      - volumeattachments
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
  - apiGroups:
      - admissionregistration.k8s.io
    resources:
      - mutatingwebhookconfigurations
      - validatingwebhookconfigurations
    verbs:
      - get
      - list
      - watch
  - apiGroups:
      - networking.k8s.io
    resources:
      - networkpolicies
      - ingressclasses
      - ingresses
    verbs:
      - get
      - list
      - watch
  - apiGroups:
      - coordination.k8s.io
    resources:
      - leases
    verbs:
      - create
      - update
      - get
      - list
      - watch
  - apiGroups:
      - rbac.authorization.k8s.io
    resources:
      - clusterrolebindings
      - clusterroles
      - rolebindings
      - roles
    verbs:
      - get
      - list
      - watch
{{- end -}}

{{- define "elasticagent.kubernetes.clusterwide.ksm.containers" -}}
extraContainers:
  - image: registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.10.0
    livenessProbe:
      httpGet:
        path: /healthz
        port: 8080
      initialDelaySeconds: 5
      timeoutSeconds: 5
    name: kube-state-metrics
    ports:
      - containerPort: 8080
        name: http-metrics
      - containerPort: 8081
        name: telemetry
    readinessProbe:
      httpGet:
        path: /
        port: 8081
      initialDelaySeconds: 5
      timeoutSeconds: 5
    securityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop:
          - ALL
      readOnlyRootFilesystem: true
      runAsNonRoot: true
      runAsUser: 65534
      seccompProfile:
        type: RuntimeDefault
{{- end -}}