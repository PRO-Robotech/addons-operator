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
      name: placeholder  # переопределяется spec.addon.name из AddonClaim
    spec:
      path: "helm-chart-sources/{{ .Vars.name }}"
      pluginName: helm-with-values
      repoURL: "https://github.com/org/helm-charts"
      version: "{{ .Vars.version }}"
      releaseName: {{ .Vars.name }}
      targetCluster: "{{ .Vars.cluster }}"
      targetNamespace: "beget-{{ .Vars.name }}"
      backend:
        type: argocd
        namespace: argocd
      variables:
        cluster_name: "{{ .Vars.cluster }}"
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

### AddonClaim (минимальный)

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonClaim
metadata:
  name: cilium
  namespace: tenant-a
spec:
  addon:
    name: cilium
  credentialRef:
    name: infra-kubeconfig
  templateRef:
    name: cilium-v1.17.4
  variables:
    version: v1.17.4
    cluster: client-cluster-01
```

### AddonClaim с valuesString

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonClaim
metadata:
  name: monitoring
  namespace: tenant-a
spec:
  addon:
    name: monitoring
  credentialRef:
    name: infra-kubeconfig
  templateRef:
    name: monitoring-v1
  variables:
    version: "1.0.0"
    cluster: client-cluster-01
  valuesString: |
    prometheus:
      replicas: 2
      retention: 30d
    grafana:
      enabled: true
```

### AddonClaim с CAPI интеграцией

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonClaim
metadata:
  name: k8s-control-plane
  namespace: tenant-a
  annotations:
    external-status/type: controlplane
spec:
  addon:
    name: k8s-cp
  credentialRef:
    name: infra-kubeconfig
  templateRef:
    name: control-plane-v1
  variables:
    version: "1.28.0"
    cluster: client-cluster-01
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
# NAME     ADDON    READY   AGE
# cilium   cilium   True    5m

# Детальные conditions
kubectl get addonclaim cilium -n tenant-a -o yaml
```

### В infra-кластере

```bash
# Addon создан
kubectl --kubeconfig=/path/to/infra-kubeconfig get addon cilium

# AddonValue создан
kubectl --kubeconfig=/path/to/infra-kubeconfig get addonvalue cilium-claim-values
```

### CAPI-совместимые поля (при наличии аннотации)

```bash
# Проверить initialized
kubectl get addonclaim k8s-control-plane -n tenant-a \
  -o jsonpath='{.status.initialized}'

# Проверить version
kubectl get addonclaim k8s-control-plane -n tenant-a \
  -o jsonpath='{.status.version}'
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
