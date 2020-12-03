# auto-replicate-secret

## Usage

Create namespace `autoops` and apply yaml resources as described below.

```yaml
# create serviceaccount
apiVersion: v1
kind: ServiceAccount
metadata:
  name: auto-replicate-secret
  namespace: autoops
---
# create clusterrole
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: auto-replicate-secret
rules:
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["watch"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "watch", "create", "update", "delete"]
---
# create clusterrolebinding
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRoleBinding
metadata:
  name: auto-replicate-secret
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: auto-replicate-secret
subjects:
  - kind: ServiceAccount
    name: auto-replicate-secret
    namespace: autoops
---
# create service
apiVersion: v1
kind: Service
metadata:
  name: auto-replicate-secret
  namespace: autoops
spec:
  ports:
    - port: 42
      name: answer
  clusterIP: None
  selector:
    k8s-app: auto-replicate-secret
---
# create statefulset
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: auto-replicate-secret
  namespace: autoops
spec:
  selector:
    matchLabels:
      k8s-app: auto-replicate-secret
  serviceName: auto-replicate-secret
  replicas: 1
  template:
    metadata:
      labels:
        k8s-app: auto-replicate-secret
    spec:
      serviceAccount: auto-replicate-secret
      containers:
        - name: auto-replicate-secret
          image: autoops/auto-replicate-secret
          imagePullPolicy: Always
```

Add a `Secret` to namespace `autoops`

Add annotation `autoops.auto-replicate-secret/enabled: "true"` if you want to replicate that secret to all namespaces

Add annotation `autoops.auto-replicate-secret/overwrite: "true"` if you want to overwrite existing `Secret`

## Credits

Guo Y.K., MIT License
