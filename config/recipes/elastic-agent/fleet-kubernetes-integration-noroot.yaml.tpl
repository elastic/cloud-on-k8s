---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: manage-agent-hostpath-permissions-{{ .Suffix }}
  namespace: {{ .Namespace }}
spec:
  selector:
    matchLabels:
      name: manage-agent-hostpath-permissions-{{ .Suffix }}
  template:
    metadata:
      labels:
        name: manage-agent-hostpath-permissions-{{ .Suffix }}
    spec:
      # This is only required when running in an selinux-enabled/Openshift environment.
      # Ensure this user has been added to the privileged scc in the correct namespace.
      # oc adm policy add-scc-to-user privileged -z elastic-agent -n elastic-apps
      # serviceAccountName: elastic-agent
      volumes:
        - hostPath:
            path: /var/lib/elastic-agent
            type: DirectoryOrCreate
          name: "agent-data"
      initContainers:
        - name: manage-agent-hostpath-permissions
          # fedora is only required when needing the `chcon` binary, if that
          # is not needed the following smaller image can be used instead:
          image: docker.io/bash:5.2.15
          # image: fedora:39
          resources:
            limits:
              cpu: 100m
              memory: 32Mi
          securityContext:
            # privileged is only required when running in an selinux-enabled/Openshift environment.
            # privileged: true
            runAsUser: 0
          volumeMounts:
            - mountPath: /var/lib/elastic-agent
              name: agent-data
          command:
          - 'bash'
          - '-e'
          - '-c'
          - |-
            # Adjust this be /var/lib/elastic-agent/YOUR-NAMESPACE/YOUR-AGENT-NAME/state
            # Multiple directories are supported for the fleet-server + agent use case.
            dirs=(
              "/var/lib/elastic-agent/{{ .Namespace }}/elastic-agent-{{ .Suffix }}/state"
              "/var/lib/elastic-agent/{{ .Namespace }}/fleet-server-{{ .Suffix }}/state"
              )
            for dir in ${dirs[@]}; do
              if [[ ! -d "${dir}" ]]
              then
                echo "creating ${dir} directory"
                mkdir -p "${dir}"
              fi
              # chcon is only required when running an an selinux-enabled/Openshift environment.
              # chcon -Rt svirt_sandbox_file_t "${dir}"
              chmod g+rw "${dir}"
              chgrp 1000 "${dir}"
              if [ -n "$(ls -A ${dir} 2>/dev/null)" ]
              then
                chgrp 1000 "${dir}"/*
                chmod g+rw "${dir}"/*
              fi
            done
      containers:
        - name: sleep
          image: docker.io/bash:5.2.15
          command: ['sleep', 'infinity']
---
apiVersion: kibana.k8s.elastic.co/v1
kind: Kibana
metadata:
  name: kibana
spec:
  version: 8.7.0
  count: 1
  elasticsearchRef:
    name: elasticsearch
  config:
    # xpack.fleet.agents.elasticsearch.hosts: ["https://elasticsearch-{{ .Suffix }}-es-http.e2e-mercury.svc:9200"]
    xpack.fleet.agents.fleet_server.hosts: ["https://fleet-server-{{ .Suffix }}-agent-http.e2e-mercury.svc:8220"]
    xpack.fleet.outputs:
    - id: eck-fleet-agent-output-elasticsearch
      is_default: true
      name: eck-elasticsearch
      type: elasticsearch
      hosts: ["https://elasticsearch-{{ .Suffix }}-es-http.e2e-mercury.svc:9200"]
      ssl:
        certificate_authorities: ["/mnt/elastic-internal/elasticsearch-association/{{ .Namespace }}/elasticsearch-{{ .Suffix }}/certs/ca.crt"]
    xpack.fleet.packages:
    - name: system
      version: latest
    - name: elastic_agent
      version: latest
    - name: fleet_server
      version: latest
    - name: kubernetes
      version: latest
    xpack.fleet.agentPolicies:
    - name: Fleet Server on ECK policy
      id: eck-fleet-server
      namespace: default
      monitoring_enabled:
      - logs
      - metrics
      unenroll_timeout: 900
      is_default_fleet_server: true
      package_policies:
      - name: fleet_server-1
        id: fleet_server-1
        package:
          name: fleet_server
    - name: Elastic Agent on ECK policy
      id: eck-agent
      namespace: default
      monitoring_enabled:
      - logs
      - metrics
      unenroll_timeout: 900
      is_default: true
      package_policies:
      - package:
          name: system
        name: system-1
      - package:
          name: kubernetes
        name: kubernetes-1
---
apiVersion: elasticsearch.k8s.elastic.co/v1
kind: Elasticsearch
metadata:
  name: elasticsearch
spec:
  version: 8.7.0
  nodeSets:
  - name: default
    count: 3
    config:
      node.store.allow_mmap: false
---
apiVersion: agent.k8s.elastic.co/v1alpha1
kind: Agent
metadata:
  name: fleet-server
spec:
  version: 8.7.0
  kibanaRef:
    name: kibana
  elasticsearchRefs:
  - name: elasticsearch
  mode: fleet
  fleetServerEnabled: true
  deployment:
    replicas: 1
    podTemplate:
      spec:
        serviceAccountName: fleet-server
        automountServiceAccountToken: true
---
apiVersion: agent.k8s.elastic.co/v1alpha1
kind: Agent
metadata: 
  name: elastic-agent
spec:
  version: 8.7.0
  kibanaRef:
    name: kibana
  fleetServerRef: 
    name: fleet-server
  mode: fleet
  daemonSet:
    podTemplate:
      spec:
        serviceAccountName: elastic-agent
        automountServiceAccountToken: true
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: fleet-server
rules:
- apiGroups: [""]
  resources:
  - pods
  - namespaces
  - nodes
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
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: fleet-server
  namespace: e2e-mercury 
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: fleet-server
subjects:
- kind: ServiceAccount
  name: fleet-server
  namespace: e2e-mercury
roleRef:
  kind: ClusterRole
  name: fleet-server
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: elastic-agent
rules:
- apiGroups: [""]
  resources:
  - pods
  - nodes
  - namespaces
  - events
  - services
  - configmaps
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
  - "/metrics"
  verbs:
  - get
- apiGroups: ["extensions"]
  resources:
    - replicasets
  verbs: 
  - "get"
  - "list"
  - "watch"
- apiGroups:
  - "apps"
  resources:
  - statefulsets
  - deployments
  - replicasets
  verbs:
  - "get"
  - "list"
  - "watch"
- apiGroups:
  - ""
  resources:
  - nodes/stats
  verbs:
  - get
- apiGroups:
  - "batch"
  resources:
  - jobs
  verbs:
  - "get"
  - "list"
  - "watch"
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: elastic-agent
  namespace: e2e-mercury
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: elastic-agent
subjects:
- kind: ServiceAccount
  name: elastic-agent
  namespace: e2e-mercury
roleRef:
  kind: ClusterRole
  name: elastic-agent
  apiGroup: rbac.authorization.k8s.io
...
