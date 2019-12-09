// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package runner

const GkeSsdProvisioner = `
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-provisioner-config 
  namespace: default 
data:
  useNodeNameOnly: "true"
  storageClassMap: |     
    e2e-default:
       hostDir: /mnt/disks/ssd0
       mountDir:  /mnt/disks/ssd0  
---
apiVersion: extensions/v1beta1
kind: DaemonSet
metadata:
  name: local-volume-provisioner
  namespace: default
  labels:
    app: local-volume-provisioner
spec:
  selector:
    matchLabels:
      app: local-volume-provisioner 
  template:
    metadata:
      labels:
        app: local-volume-provisioner
    spec:
      serviceAccountName: local-storage-admin
	  initContainers:
	  - name: install
		image: busybox
		command: ['sh', '-c', 'for i in $(seq 1 10); do mkdir -p "/mnt/disks/ssd0/disk-$i"; done']
		volumeMounts:
		  - mountPath:  /mnt/disks 
		    name: e2e-default
      containers:
        - image: "quay.io/external_storage/local-volume-provisioner:v2.2.0"
          imagePullPolicy: "Always"
          name: provisioner 
          securityContext:
            privileged: true
          env:
          - name: MY_NODE_NAME
            valueFrom:
              fieldRef:
                fieldPath: spec.nodeName
          volumeMounts:
            - mountPath: /etc/provisioner/config 
              name: provisioner-config
              readOnly: true             
            - mountPath:  /mnt/disks 
              name: e2e-default 
      volumes:
        - name: provisioner-config
          configMap:
            name: local-provisioner-config         
        - name: e2e-default
          hostPath:
            path: /mnt/disks 
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: local-storage-admin
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: local-storage-provisioner-pv-binding
  namespace: default
subjects:
- kind: ServiceAccount
  name: local-storage-admin
  namespace: default
roleRef:
  kind: ClusterRole
  name: system:persistent-volume-provisioner
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: local-storage-provisioner-node-clusterrole
  namespace: default
rules:
- apiGroups:
  - extensions
  resources:
  - podsecuritypolicies
  resourceNames:
  - gce.privileged
  verbs:
  - use
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: local-storage-provisioner-node-binding
  namespace: default
subjects:
- kind: ServiceAccount
  name: local-storage-admin
  namespace: default
roleRef:
  kind: ClusterRole
  name: local-storage-provisioner-node-clusterrole
  apiGroup: rbac.authorization.k8s.io
`
