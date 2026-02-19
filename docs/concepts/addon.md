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
| `chart` | Нет* | Имя Helm chart (для Helm репозиториев) |
| `path` | Нет* | Путь к директории с чартом (для Git репозиториев) |
| `repoURL` | Да | URL Helm или Git репозитория |
| `version` | Да | Версия chart или Git ревизия (branch, tag, commit) |
| `pluginName` | Нет | Имя ArgoCD Config Management Plugin (заменяет встроенный Helm) |
| `releaseName` | Нет | Переопределение имени Helm release |

\* Должен быть указан либо `chart`, либо `path`, но не оба одновременно.

### Целевая конфигурация

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `targetCluster` | Да | Целевой кластер (см. форматы ниже) |
| `targetNamespace` | Да | Namespace для развёртывания |

#### targetCluster

Поддерживается три формата:

| Формат | Пример | Результат в ArgoCD |
|--------|--------|-------------------|
| Специальное значение | `in-cluster` | `destination.server: https://kubernetes.default.svc` |
| URL кластера | `https://k8s.example.com:6443` | `destination.server: <URL>` |
| Имя кластера | `production-cluster` | `destination.name: <name>` |

При использовании имени кластера, кластер должен быть предварительно зарегистрирован в ArgoCD.

### Конфигурация Backend

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `backend.type` | Нет | Тип backend (по умолчанию: `argocd`) |
| `backend.namespace` | Да | Namespace где создаётся Application |
| `backend.project` | Нет | Argo CD project (по умолчанию: `default`) |
| `backend.syncPolicy` | Нет | Политика авто-синхронизации Argo CD |
| `backend.syncPolicy.managedNamespaceMetadata` | Нет | Labels и annotations для target namespace |
| `backend.ignoreDifferences` | Нет | Правила игнорирования drift для ресурсов |

### Выбор Values

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `valuesSelectors` | Нет | Список селекторов для AddonValue ресурсов |
| `valuesSources` | Нет | Внешние источники данных (любые Kubernetes ресурсы) |
| `variables` | Нет | Переменные для рендеринга шаблонов |

### Удаление ресурсов

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `finalizer` | Нет | Каскадное удаление ресурсов при удалении Application |

Когда `finalizer: true`, при удалении Addon (и, соответственно, Argo CD Application) все ресурсы, созданные этим Application, будут удалены из кластера. Без этого поля удаляется только сам объект Application.

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

  # Каскадное удаление ресурсов при удалении Addon
  finalizer: true

  # Конфигурация Argo CD
  backend:
    type: argocd
    namespace: argocd
    project: infrastructure
    syncPolicy:
      automated:
        prune: true
        selfHeal: true
      syncOptions:
        - CreateNamespace=true
      # Labels/annotations для создаваемого namespace
      managedNamespaceMetadata:
        labels:
          environment: production
        annotations:
          description: "Managed by addon-operator"
    # Игнорировать drift для определённых полей
    ignoreDifferences:
      - group: admissionregistration.k8s.io
        kind: ValidatingWebhookConfiguration
        jsonPointers:
          - /webhooks/0/failurePolicy

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

### Пример с Git репозиторием

Для развёртывания чарта из Git репозитория используйте `path` вместо `chart`:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: my-app
spec:
  # Путь к чарту в Git репозитории
  path: charts/my-app
  repoURL: https://github.com/org/helm-charts.git
  version: main  # branch, tag или commit SHA

  targetCluster: in-cluster
  targetNamespace: my-app

  backend:
    type: argocd
    namespace: argocd
```

### Пример с Config Management Plugin

Для использования [ArgoCD CMP](https://argo-cd.readthedocs.io/en/stable/operator-manual/config-management-plugins/) вместо встроенного Helm укажите `pluginName`. Values передаются через переменную окружения `HELM_VALUES` (base64):

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: secrets-app
spec:
  chart: my-chart
  repoURL: https://example.com/charts
  version: "1.0.0"
  targetCluster: in-cluster
  targetNamespace: my-app

  # Config Management Plugin вместо Helm
  pluginName: helm-secrets
  releaseName: custom-release

  backend:
    type: argocd
    namespace: argocd
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

| Поле | Описание |
|------|----------|
| `deployed` | Addon был успешно развёрнут хотя бы один раз (latching — никогда не сбрасывается) |
| `applicationRef` | Ссылка на созданный Argo CD Application |
| `observedGeneration` | Последнее обработанное поколение spec |
| `valuesHash` | Хеш итоговых merged values |
| `phaseValuesSelector` | Динамические селекторы от AddonPhase |
| `conditions` | Текущее состояние Addon (см. Conditions выше) |

```yaml
status:
  deployed: true  # Был развёрнут хотя бы раз
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

## Стабилизация Values

При **первом** создании Addon контроллер не создаёт Argo CD Application немедленно.
Вместо этого он вычисляет хеш итоговых values и ждёт, пока хеш не стабилизируется
(совпадёт на двух последовательных reconcile циклах).

Это защищает от race condition: при одновременном создании Addon, AddonValue и AddonPhase
через один Helm chart или манифест, informer cache может ещё не видеть все ресурсы
на первом reconcile. Стабилизация гарантирует, что Application получит полный набор values.

### Поведение

- Condition `Progressing` с reason `WaitingForStableValues` — ожидание стабилизации
- Типичная задержка: 1-2 секунды (один дополнительный reconcile цикл)
- После создания Application стабилизация не применяется — обновления values применяются немедленно

### Диагностика

```bash
kubectl get addon my-addon -o jsonpath='{.status.conditions[?(@.reason=="WaitingForStableValues")]}'
```

Если condition WaitingForStableValues сохраняется дольше 10 секунд, values продолжают меняться
между reconcile циклами (например, AddonPhase постоянно обновляет selectors).

## Пауза Reconciliation

Для отладки можно приостановить reconciliation Addon, чтобы вручную редактировать
Application в ArgoCD UI/CLI без перезаписи контроллером.

### Использование

```bash
# Поставить на паузу
kubectl annotate addon cilium addons.in-cloud.io/paused=true

# Снять с паузы
kubectl annotate addon cilium addons.in-cloud.io/paused-
```

### Поведение при паузе

| Аспект | Поведение |
|--------|-----------|
| Addon → Application | Остановлено (контроллер не перезаписывает) |
| ArgoCD sync | Продолжает работать |
| Application в ArgoCD | Можно редактировать вручную |
| Status conditions | `Ready=False`, `Progressing=False`, Reason=Paused |
| Delete | Работает (финализатор срабатывает) |

При паузе status показывает:

```yaml
status:
  conditions:
    - type: Ready
      status: "False"
      reason: Paused
      message: "Reconciliation is paused"
    - type: Progressing
      status: "False"
      reason: Paused
      message: "Reconciliation is paused"
    - type: Degraded
      status: "False"  # Пауза - не ошибка
```

## Зависимости

Используйте `initDependencies` для блокировки развёртывания до готовности зависимостей:

```yaml
spec:
  initDependencies:
    - name: cert-manager
      criteria:
        - jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
```

Addon будет иметь `DependenciesMet=False` с reason `WaitingForDependencies` пока все зависимости не удовлетворят свои criteria.

## Статус развёртывания (Deployed)

Поле `status.deployed` — latching-флаг, который устанавливается в `true` после первого
успешного развёртывания (Application в состоянии Synced + Healthy) и **никогда не сбрасывается**.

### Зачем нужен

Condition `Ready` отражает **текущее** состояние: Addon может быть Ready, затем стать Degraded
при обновлении, и снова Ready. Поле `deployed` отвечает на другой вопрос:
**«был ли этот Addon хотя бы раз успешно развёрнут?»**

### Использование в criteria

Используйте `$.status.deployed` в initDependencies или AddonPhase criteria для проверки
факта первого развёртывания:

```yaml
spec:
  initDependencies:
    - name: cert-manager
      criteria:
        - jsonPath: $.status.deployed
          operator: Equal
          value: true
```

### kubectl

Поле отображается в выводе `kubectl get addon`:

```
NAME      CHART     VERSION   READY   DEPLOYED   AGE
cilium    cilium    1.14.5    True    true       5m
my-app    my-app    2.0.0     False   true       10m   # был развёрнут, сейчас нездоров
new-app   new-app   1.0.0     False   <none>     30s   # ещё не был развёрнут
```

## Удаление Addon

При удалении Addon контроллер выполняет безопасную очистку:

1. Контроллер отправляет запрос на удаление ArgoCD Application
2. Контроллер **дожидается полного удаления** Application (опрос каждые 5 секунд)
3. Только после подтверждения удаления Application — снимает финализатор с Addon
4. Addon удаляется из кластера

Если у Application установлен финализатор `resources-finalizer.argocd.argoproj.io` (через `spec.finalizer: true`), ArgoCD сначала удалит все managed-ресурсы (Deployments, Services, ConfigMaps и др.), и только потом удалит сам объект Application. Контроллер Addon терпеливо ждёт завершения этого процесса.

### Диагностика

В логах контроллера при ожидании удаления будет:

```
INFO  Waiting for ArgoCD Application to be fully deleted  {"name": "cilium", "namespace": "argocd"}
```

После завершения:

```
INFO  ArgoCD Application deleted  {"name": "cilium", "namespace": "argocd"}
```

## Связанные ресурсы

- [AddonValue](addon-value.md) — конфигурационные values
- [AddonPhase](addon-phase.md) — условные фичи
- [valuesSources](values-sources.md) — извлечение внешних данных
