# Addon

Ресурс Addon представляет развёртывание Helm chart, управляемое через Argo CD.

## Обзор

Addon:
- Определяет **что** развернуть (Helm chart, репозиторий, версия)
- Определяет **где** развернуть (целевой кластер и namespace)
- Определяет **как** выбрать конфигурационные values (valuesSelectors)
- Создаёт и управляет Argo CD Application

## Поля Spec

### Конфигурация Chart

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `chart` | Да | Имя Helm chart |
| `repoURL` | Да | URL Helm репозитория |
| `version` | Да | Версия chart |

### Целевая конфигурация

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `targetCluster` | Да | Целевой кластер (`in-cluster` или URL кластера) |
| `targetNamespace` | Да | Namespace для развёртывания |

### Конфигурация Backend

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `backend.type` | Нет | Тип backend (по умолчанию: `argocd`) |
| `backend.namespace` | Да | Namespace где создаётся Application |
| `backend.project` | Нет | Argo CD project (по умолчанию: `default`) |
| `backend.syncPolicy` | Нет | Политика авто-синхронизации Argo CD |

### Выбор Values

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `valuesSelectors` | Нет | Список селекторов для AddonValue ресурсов |
| `valuesSources` | Нет | Внешние источники данных (любые Kubernetes ресурсы) |
| `variables` | Нет | Переменные для рендеринга шаблонов |

### Зависимости

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `initDependencies` | Нет | Блокировать развёртывание до удовлетворения зависимостей |

## Пример

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: cilium  # Addon cluster-scoped, без namespace
spec:
  # Что развернуть
  chart: cilium
  repoURL: https://helm.cilium.io
  version: "1.14.5"

  # Где развернуть
  targetCluster: in-cluster
  targetNamespace: kube-system

  # Конфигурация Argo CD
  backend:
    type: argocd
    namespace: argocd
    project: infrastructure

  # Выбор values (порядок приоритета: 0 наименьший, 99 наибольший)
  valuesSelectors:
    - name: defaults
      priority: 0
      matchLabels:
        addons.in-cloud.io/addon: cilium
        addons.in-cloud.io/layer: defaults
    - name: custom
      priority: 50
      matchLabels:
        addons.in-cloud.io/addon: cilium
        addons.in-cloud.io/layer: custom

  # Переменные для рендеринга шаблонов
  variables:
    cluster_name: production
    region: us-east-1
```

## Status

### Conditions

| Тип | Значение |
|-----|----------|
| `Ready` | Addon полностью reconciled и здоров |
| `Progressing` | Выполняется reconciliation |
| `Degraded` | Произошла ошибка |
| `ApplicationCreated` | Argo CD Application создан |
| `ValuesResolved` | Все values успешно разрешены |
| `DependenciesMet` | Все init зависимости удовлетворены |
| `Synced` | Статус синхронизации Application |
| `Healthy` | Статус здоровья Application |

### Поля Status

```yaml
status:
  applicationRef:
    name: cilium
    namespace: argocd
  observedGeneration: 1
  valuesHash: "abc123"
  phaseValuesSelector: []  # Заполняется AddonPhase
  conditions:
    - type: Ready
      status: "True"
      reason: FullyReconciled
    - type: Progressing
      status: "False"
    - type: Degraded
      status: "False"
    - type: DependenciesMet
      status: "True"
      reason: AllDependenciesMet
    - type: ValuesResolved
      status: "True"
      reason: ValuesResolved
    - type: ApplicationCreated
      status: "True"
      reason: ApplicationCreated
    - type: Synced
      status: "True"
      reason: Synced
    - type: Healthy
      status: "True"
      reason: Healthy
```

## Выбор Values

Values выбираются и объединяются из ресурсов AddonValue:

1. Контроллер находит все AddonValue, соответствующие `matchLabels` каждого селектора
2. Values объединяются в **порядке приоритета** (меньший первым)
3. Values с большим приоритетом перезаписывают values с меньшим
4. Итоговые объединённые values передаются в Application

```
Приоритет 0:  { replicas: 1, memory: "128Mi" }
Приоритет 50: { replicas: 3 }
Приоритет 99: { memory: "512Mi" }
─────────────────────────────────────────────
Результат:    { replicas: 3, memory: "512Mi" }
```

## Зависимости

Используйте `initDependencies` для блокировки развёртывания до готовности зависимостей:

```yaml
spec:
  initDependencies:
    - name: cert-manager
      criteria:
        - jsonPath: /status/conditions/0/status
          operator: Equal
          value: "True"
```

Addon будет иметь `DependenciesMet=False` с reason `WaitingForDependencies` пока все зависимости не удовлетворят свои criteria.

## Связанные ресурсы

- [AddonValue](addon-value.md) — конфигурационные values
- [AddonPhase](addon-phase.md) — условные фичи
- [valuesSources](values-sources.md) — извлечение внешних данных
