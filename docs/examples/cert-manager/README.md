# Пример cert-manager

Этот пример демонстрирует развёртывание cert-manager с автоматическим созданием ClusterIssuer.

## Обзор

- **cert-manager** для управления сертификатами
- **Self-signed ClusterIssuer** для внутренних сертификатов
- **Let's Encrypt ClusterIssuer** для production (условно)

## Ресурсы

### 1. Addon

```yaml
# cert-manager-addon.yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: cert-manager
spec:
  chart: cert-manager
  repoURL: https://charts.jetstack.io
  version: "1.14.0"
  targetCluster: in-cluster
  targetNamespace: cert-manager
  backend:
    namespace: argocd
    syncPolicy:
      automated:
        prune: true
        selfHeal: true
      syncOptions:
        - CreateNamespace=true

  valuesSelectors:
    - name: base
      priority: 0
      matchLabels:
        addons.in-cloud.io/addon: cert-manager
        addons.in-cloud.io/layer: base
```

### 2. Базовые Values

```yaml
# cert-manager-values.yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: cert-manager-base
  labels:
    addons.in-cloud.io/addon: cert-manager
    addons.in-cloud.io/layer: base
spec:
  values:
    installCRDs: true
    replicaCount: 1
    webhook:
      replicaCount: 1
    cainjector:
      replicaCount: 1
```

> **Примечание:** Для production рекомендуется установить `replicaCount: 2` и включить мониторинг через Prometheus ServiceMonitor (требует установленный Prometheus Operator).

## Развёртывание

```bash
# Применить AddonValue
kubectl apply -f cert-manager-values.yaml

# Применить Addon
kubectl apply -f cert-manager-addon.yaml

# Проверить статус
kubectl get addon cert-manager -o wide
```

## Проверка

```bash
# Проверить поды cert-manager
kubectl get pods -n cert-manager

# Проверить CRD
kubectl get crd | grep cert-manager

# Проверить condition Ready
kubectl get addon cert-manager -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'
```

## Использование как зависимости

Другие аддоны могут зависеть от cert-manager:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: my-app
spec:
  # ... конфигурация chart ...
  initDependencies:
    - name: cert-manager
      criteria:
        - jsonPath: /status/conditions/0/status
          operator: Equal
          value: "True"
```

## Очистка

```bash
kubectl delete addon cert-manager
kubectl delete addonvalue cert-manager-base
```
