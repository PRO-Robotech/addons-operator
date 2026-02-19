# Мультикластерный пример

Этот пример демонстрирует развёртывание аддона Cilium в infra-кластер через AddonClaim.

## Ресурсы

### AddonTemplate

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonTemplate
metadata:
  name: cilium-v1.17.4
  labels:
    name.addons.in-cloud.io: cilium
    version.addons.in-cloud.io: v1.17.4
spec:
  template: |
    apiVersion: addons.in-cloud.io/v1alpha1
    kind: Addon
    metadata:
      name: {{ .Values.spec.name }}
    spec:
      path: "helm-chart-sources/{{ .Values.spec.name }}"
      pluginName: helm-with-values
      repoURL: "https://github.com/org/helm-charts"
      version: "{{ .Values.spec.version }}"
      releaseName: {{ .Values.spec.name }}
      targetCluster: "{{ .Values.spec.cluster }}"
      targetNamespace: "beget-{{ .Values.spec.name }}"
      backend:
        type: argocd
        namespace: argocd
      variables:
        cluster_name: "{{ .Values.spec.cluster }}"
```

### Credential Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: infra-kubeconfig
  namespace: tenant-a
type: Opaque
data:
  value: <base64-encoded kubeconfig>
```

### AddonClaim с dependency

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonClaim
metadata:
  name: cilium
  namespace: tenant-a
spec:
  name: cilium
  version: v1.17.4
  cluster: client-cluster-01
  credentialRef:
    name: infra-kubeconfig
  templateRef:
    name: cilium-v1.17.4
  dependency: true
```

### AddonClaim с valuesString

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonClaim
metadata:
  name: monitoring
  namespace: tenant-a
spec:
  name: monitoring
  version: "1.0.0"
  cluster: client-cluster-01
  credentialRef:
    name: infra-kubeconfig
  templateRef:
    name: monitoring-v1
  valuesString: |
    prometheus:
      replicas: 2
      retention: 30d
    grafana:
      enabled: true
```

## Развёртывание

```bash
# 1. Создать namespace
kubectl create namespace tenant-a

# 2. Создать kubeconfig Secret
kubectl create secret generic infra-kubeconfig \
  --namespace=tenant-a \
  --from-file=value=/path/to/infra-cluster-kubeconfig

# 3. Создать AddonTemplate
kubectl apply -f addontemplate.yaml

# 4. Создать AddonClaim
kubectl apply -f addonclaim.yaml
```

## Проверка

### В system-кластере

```bash
# Статус AddonClaim
kubectl get addonclaim -n tenant-a
# NAME     ADDON    VERSION    CLUSTER              READY   AGE
# cilium   cilium   v1.17.4    client-cluster-01    True    5m

# Детальные conditions
kubectl get addonclaim cilium -n tenant-a -o yaml
```

### В infra-кластере

```bash
# Addon создан
kubectl --kubeconfig=/path/to/infra-kubeconfig get addon cilium
# NAME     CHART   VERSION    READY   DEPLOYED   AGE
# cilium           v1.17.4    True    true       4m

# AddonValue создан
kubectl --kubeconfig=/path/to/infra-kubeconfig get addonvalue cilium-claim-values
```

## Cleanup

```bash
# Удаление AddonClaim (автоматически удаляет ресурсы из infra-кластера)
kubectl delete addonclaim cilium -n tenant-a

# Удаление шаблона
kubectl delete addontemplate cilium-v1.17.4

# Удаление Secret
kubectl delete secret infra-kubeconfig -n tenant-a
```
