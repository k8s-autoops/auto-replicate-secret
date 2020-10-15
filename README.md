# template-autoops-statefulset

## Usage

Create namespace `autoops` and apply yaml resources as described below.

```yaml
# create serviceaccount
apiVersion: v1
kind: ServiceAccount
metadata:
  name: template-autoops-statefulset
  namespace: autoops
---
# create clusterrole
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: template-autoops-statefulset
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["list"]
---
# create clusterrolebinding
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRoleBinding
metadata:
  name: template-autoops-statefulset
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: template-autoops-statefulset
subjects:
  - kind: ServiceAccount
    name: template-autoops-statefulset
    namespace: autoops
---
# create service
apiVersion: v1
kind: Service
metadata:
  name: template-autoops-statefulset
  namespace: autoops
spec:
  ports:
    - port: 42
      name: answer
  clusterIP: None
  selector:
    app: template-autoops-statefulset
---
# create statefulset
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: template-autoops-statefulset
  namespace: autoops
spec:
  selector:
    matchLabels:
      k8s-app: template-autoops-statefulset
  serviceName: template-autoops-statefulset
  replicas: 1
  template:
    metadata:
      labels:
        k8s-app: template-autoops-statefulset
    spec:
      serviceAccount: template-autoops-statefulset
      containers:
        - name: template-autoops-statefulset
          image: autoops/template-autoops-statefulset
          imagePullPolicy: Always
```

## Credits

Guo Y.K., MIT License
