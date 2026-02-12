# Примеры

Этот каталог содержит примеры реальных развёртываний с использованием Addon Operator.

## Доступные примеры

| Пример | Описание |
|--------|----------|
| [Cilium](cilium/) | Развёртывание CNI с условной TLS фичей |
| [cert-manager](cert-manager/) | Управление сертификатами с ClusterIssuer |
| [Многоуровневое приложение](multi-tier-app/) | Стек приложений с упорядоченными зависимостями |

## Паттерны примеров

### Базовый Addon

Минимальное развёртывание аддона:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: podinfo
spec:
  chart: podinfo
  repoURL: https://stefanprodan.github.io/podinfo
  version: "6.5.0"
  targetCluster: in-cluster
  targetNamespace: default
  backend:
    namespace: argocd
```

### Addon с Values

Использование AddonValue для конфигурации:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: podinfo-config
  labels:
    addons.in-cloud.io/addon: podinfo
spec:
  values:
    replicaCount: 3
---
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: podinfo
spec:
  chart: podinfo
  repoURL: https://stefanprodan.github.io/podinfo
  version: "6.5.0"
  targetCluster: in-cluster
  targetNamespace: default
  backend:
    namespace: argocd
  valuesSelectors:
    - name: config
      priority: 0
      matchLabels:
        addons.in-cloud.io/addon: podinfo
```

### Addon с зависимостями

Ожидание другого аддона перед развёртыванием:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: my-app
spec:
  chart: podinfo
  repoURL: https://stefanprodan.github.io/podinfo
  version: "6.5.0"
  targetCluster: in-cluster
  targetNamespace: default
  backend:
    namespace: argocd
  initDependencies:
    - name: cert-manager
      criteria:
        - jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
```

### Условные фичи с AddonPhase

Активация values на основе состояния кластера:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonPhase
metadata:
  name: my-app
spec:
  rules:
    - name: enable-tls
      criteria:
        - source:
            apiVersion: addons.in-cloud.io/v1alpha1
            kind: Addon
            name: cert-manager
          jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
      selector:
        name: tls-feature
        priority: 10
        matchLabels:
          addons.in-cloud.io/addon: my-app
          addons.in-cloud.io/feature.tls: "true"
```

## Добавление примеров

Чтобы добавить новый пример:

1. Создайте каталог в `docs/examples/`
2. Добавьте `README.md` с:
   - Обзором
   - Определениями ресурсов
   - Инструкциями по развёртыванию
   - Шагами проверки
   - Инструкциями по очистке
3. Обновите этот индекс
