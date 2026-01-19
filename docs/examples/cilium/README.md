# Пример Cilium CNI

Этот пример демонстрирует развёртывание Cilium CNI с условной активацией фич через AddonPhase.

## Обзор

- **Cilium** как CNI плагин
- **Hubble** observability включён по умолчанию
- **TLS сертификаты** включаются когда cert-manager готов

## Ресурсы

### 1. Addon

```yaml
# cilium-addon.yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: cilium
spec:
  chart: cilium
  repoURL: https://helm.cilium.io
  version: "1.14.5"
  targetCluster: in-cluster
  targetNamespace: kube-system
  backend:
    namespace: argocd

  valuesSelectors:
    - name: defaults
      priority: 0
      matchLabels:
        addons.in-cloud.io/addon: cilium
        addons.in-cloud.io/layer: defaults
    - name: hubble
      priority: 10
      matchLabels:
        addons.in-cloud.io/addon: cilium
        addons.in-cloud.io/feature.hubble: "true"

  variables:
    cluster_name: production
    cluster_id: "1"
```

> **Примечание:** Cilium — это CNI плагин. Развёртывание в существующем кластере требует осторожности и может повлиять на сетевое взаимодействие. Этот пример предназначен как справочник.

### 2. Values по умолчанию (AddonValue)

```yaml
# cilium-defaults.yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: cilium-defaults
  labels:
    addons.in-cloud.io/addon: cilium
    addons.in-cloud.io/layer: defaults
spec:
  values:
    cluster:
      name: "{{ .Variables.cluster_name }}"
      id: "{{ .Variables.cluster_id }}"
    ipam:
      mode: kubernetes
    operator:
      replicas: 2
    kubeProxyReplacement: strict
```

### 3. Values фичи Hubble (AddonValue)

```yaml
# cilium-hubble.yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: cilium-hubble
  labels:
    addons.in-cloud.io/addon: cilium
    addons.in-cloud.io/feature.hubble: "true"
spec:
  values:
    hubble:
      enabled: true
      relay:
        enabled: true
      ui:
        enabled: true
```

### 4. Values фичи сертификатов (AddonValue)

```yaml
# cilium-certificates.yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: cilium-certificates
  labels:
    addons.in-cloud.io/addon: cilium
    addons.in-cloud.io/feature.certificates: "true"
spec:
  values:
    hubble:
      tls:
        enabled: true
        auto:
          enabled: true
          method: certmanager
          certManagerIssuerRef:
            group: cert-manager.io
            kind: ClusterIssuer
            name: selfsigned-issuer
```

### 5. AddonPhase для условных фич

```yaml
# cilium-phase.yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonPhase
metadata:
  name: cilium  # cluster-scoped
spec:
  rules:
    # Фича сертификатов — активируется когда cert-manager готов
    - name: certificates
      criteria:
        - source:
            apiVersion: addons.in-cloud.io/v1alpha1
            kind: Addon
            name: cert-manager
          jsonPath: /status/conditions/0/status
          operator: Equal
          value: "True"
      selector:
        name: certificates
        priority: 20
        matchLabels:
          addons.in-cloud.io/addon: cilium
          addons.in-cloud.io/feature.certificates: "true"
```

## Развёртывание

### Применение по порядку

```bash
# 1. Создать AddonValues первыми
kubectl apply -f cilium-defaults.yaml
kubectl apply -f cilium-hubble.yaml
kubectl apply -f cilium-certificates.yaml

# 2. Создать Addon
kubectl apply -f cilium-addon.yaml

# 3. Создать AddonPhase
kubectl apply -f cilium-phase.yaml
```

### Проверка развёртывания

```bash
# Проверить статус Addon
kubectl get addon cilium -o yaml

# Проверить правила AddonPhase
kubectl get addonphase cilium -o yaml

# Проверить Argo CD Application
kubectl get application -n argocd cilium
```

## Процесс активации фич

```
Начальное состояние (cert-manager не готов):
┌──────────┐        ┌────────────────┐
│  Addon   │───────▶│ cilium-defaults│ приоритет: 0
│  cilium  │───────▶│ cilium-hubble  │ приоритет: 10
└──────────┘        └────────────────┘

После готовности cert-manager:
┌──────────┐        ┌────────────────────┐
│  Addon   │───────▶│ cilium-defaults    │ приоритет: 0
│  cilium  │───────▶│ cilium-hubble      │ приоритет: 10
│          │───────▶│ cilium-certificates│ приоритет: 20 (из AddonPhase)
└──────────┘        └────────────────────┘
```

## Очистка

```bash
kubectl delete addonphase cilium
kubectl delete addon cilium
kubectl delete addonvalue cilium-defaults cilium-hubble cilium-certificates
```
