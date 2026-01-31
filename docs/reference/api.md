# Справочник API

Этот документ описывает Custom Resource Definitions (CRD) для Addon Operator.

## Обзор ресурсов

| Ресурс | Область | Описание |
|--------|---------|----------|
| [Addon](#addon) | Cluster | Основной ресурс для управления Helm развёртываниями |
| [AddonValue](#addonvalue) | Cluster | Хранит фрагменты Helm values |
| [AddonPhase](#addonphase) | Cluster | Условная активация селекторов values |

## Addon

`addons.in-cloud.io/v1alpha1`

Addon — основной ресурс для управления Helm-based развёртываниями. Он агрегирует values из AddonValue ресурсов, генерирует Argo CD Application и отслеживает статус развёртывания.

### AddonSpec

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `chart` | string | Нет* | Имя Helm chart (для Helm репозиториев) |
| `path` | string | Нет* | Путь к директории с чартом в Git репозитории |
| `repoURL` | string | Да | URL Helm или Git репозитория (должен начинаться с http:// или https://) |
| `version` | string | Да | Версия chart или Git ревизия (branch, tag, commit) |

\* Должен быть указан либо `chart`, либо `path`, но не оба одновременно.
| `targetCluster` | string | Да | Целевой кластер: `in-cluster`, URL (`https://...`), или имя кластера в ArgoCD |
| `targetNamespace` | string | Да | Namespace для ресурсов chart |
| `backend` | [BackendSpec](#backendspec) | Да | Конфигурация бэкенда развёртывания |
| `valuesSelectors` | [][ValuesSelector](#valuesselector) | Нет | Статические селекторы для AddonValue ресурсов |
| `valuesSources` | [][ValueSource](#valuesource) | Нет | Внешние источники для извлечения values |
| `variables` | map[string]string | Нет | Переменные для рендеринга Go шаблонов |
| `pluginName` | string | Нет | Имя ArgoCD Config Management Plugin (вместо встроенного Helm) |
| `releaseName` | string | Нет | Переопределение имени Helm release |
| `initDependencies` | [][Dependency](#dependency) | Нет | Зависимости, которые должны быть готовы первыми |
| `finalizer` | *bool | Нет | Каскадное удаление ресурсов при удалении Application |

### AddonStatus

| Поле | Тип | Описание |
|------|-----|----------|
| `observedGeneration` | int64 | Последняя обработанная spec.generation |
| `phaseValuesSelector` | [][ValuesSelector](#valuesselector) | Динамические селекторы из AddonPhase |
| `applicationRef` | [ApplicationRef](#applicationref) | Ссылка на Argo CD Application |
| `valuesHash` | string | Хеш объединённых values |
| `conditions` | []Condition | Conditions текущего состояния |

### Status Conditions

| Тип | Описание |
|-----|----------|
| `Ready` | Общее здоровье аддона (True когда полностью работает) |
| `Progressing` | Выполняется reconciliation |
| `Degraded` | Произошла невосстановимая ошибка |
| `DependenciesMet` | Все зависимости удовлетворены |
| `ValuesResolved` | Агрегация values завершена |
| `ApplicationCreated` | Argo CD Application существует |
| `Synced` | Application синхронизирован |
| `Healthy` | Application здоров |

### Config Management Plugin (pluginName)

Когда указано `pluginName`, вместо встроенного Helm source используется [ArgoCD Config Management Plugin](https://argo-cd.readthedocs.io/en/stable/operator-manual/config-management-plugins/). В этом режиме:

- Values передаются через переменную окружения `HELM_VALUES` (base64-encoded YAML)
- `source.helm` не заполняется (вместо него используется `source.plugin`)

### Переопределение Release Name (releaseName)

Поле `releaseName` переопределяет имя Helm release:

| Режим | Поведение |
|-------|-----------|
| Helm (без `pluginName`) | Устанавливается в `source.helm.releaseName` |
| Plugin (с `pluginName`) | Передаётся как переменная окружения `RELEASE_NAME` |

### Каскадное удаление ресурсов (finalizer)

Когда `finalizer: true`, на Argo CD Application устанавливается финалайзер `resources-finalizer.argocd.argoproj.io`. При удалении Application Argo CD сначала удалит все созданные им ресурсы (Deployments, Services и т.д.), и только затем удалит сам объект Application.

Без этого финалайзера (по умолчанию) удаляется только объект Application, а ресурсы в кластере остаются.

### Пример (Helm репозиторий)

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: prometheus
spec:
  chart: kube-prometheus-stack
  repoURL: https://prometheus-community.github.io/helm-charts
  version: "55.5.0"
  targetCluster: in-cluster
  targetNamespace: monitoring
  finalizer: true  # удалить ресурсы при удалении Application
  backend:
    type: argocd
    namespace: argocd
    project: default
    syncPolicy:
      automated:
        prune: true
        selfHeal: true
  valuesSelectors:
    - name: base
      priority: 0
      matchLabels:
        addons.in-cloud.io/addon: prometheus
        addons.in-cloud.io/layer: base
    - name: production
      priority: 10
      matchLabels:
        addons.in-cloud.io/addon: prometheus
        addons.in-cloud.io/environment: production
  variables:
    cluster_name: production-cluster
  initDependencies:
    - name: cert-manager
      criteria:
        - jsonPath: /status/conditions/0/status
          operator: Equal
          value: "True"
```

### Пример (Git репозиторий)

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: my-app
spec:
  path: charts/my-app
  repoURL: https://github.com/org/helm-charts.git
  version: main
  targetCluster: in-cluster
  targetNamespace: my-app
  backend:
    type: argocd
    namespace: argocd
```

### Пример (внешний кластер по имени)

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: monitoring
spec:
  chart: kube-prometheus-stack
  repoURL: https://prometheus-community.github.io/helm-charts
  version: "55.5.0"
  # Использование имени кластера, зарегистрированного в ArgoCD
  targetCluster: production-cluster
  targetNamespace: monitoring
  backend:
    type: argocd
    namespace: argocd
```

### Пример (Config Management Plugin)

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
  pluginName: helm-secrets       # использовать CMP вместо Helm
  releaseName: custom-release    # переопределить имя release
  backend:
    type: argocd
    namespace: argocd
```

### Пример (Helm с переопределением Release Name)

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: ingress
spec:
  chart: ingress-nginx
  repoURL: https://kubernetes.github.io/ingress-nginx
  version: "4.8.3"
  targetCluster: in-cluster
  targetNamespace: ingress-nginx
  releaseName: nginx  # по умолчанию используется имя Addon
  backend:
    type: argocd
    namespace: argocd
```

---

## AddonValue

`addons.in-cloud.io/v1alpha1`

AddonValue хранит фрагмент Helm values, который может выбираться Addon'ом через сопоставление меток.

### AddonValueSpec

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `values` | object | Да | Фрагмент Helm values (произвольный YAML/JSON) |

### Соглашения по меткам

| Метка | Назначение | Пример |
|-------|------------|--------|
| `addons.in-cloud.io/addon` | Связь с аддоном | `prometheus` |
| `addons.in-cloud.io/layer` | Слой values | `defaults`, `custom`, `immutable` |
| `addons.in-cloud.io/feature.<name>` | Флаг фичи | `true` |

### Поддержка шаблонов

Values поддерживают Go шаблоны:

| Синтаксис | Описание |
|-----------|----------|
| `{{ .Variables.key }}` | Доступ к переменным аддона |
| `{{ .Values.key }}` | Доступ к values из valuesSources |

### Пример

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: prometheus-base
  labels:
    addons.in-cloud.io/addon: prometheus
    addons.in-cloud.io/layer: base
spec:
  values:
    alertmanager:
      enabled: true
    prometheus:
      prometheusSpec:
        replicas: 2
        externalLabels:
          cluster: "{{ .Variables.cluster_name }}"
```

---

## AddonPhase

`addons.in-cloud.io/v1alpha1`

AddonPhase — движок правил для условной активации селекторов values. Он вычисляет criteria по состоянию кластера и инъектирует совпавшие селекторы в status.phaseValuesSelector связанного Addon'а.

**Связь**: AddonPhase имеет связь 1:1 с Addon по имени. AddonPhase с именем "foo" управляет селекторами для Addon с именем "foo".

### AddonPhaseSpec

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `rules` | [][PhaseRule](#phaserule) | Да | Правила для активации селекторов (мин. 1) |

### PhaseRule

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `name` | string | Да | Идентификатор правила |
| `criteria` | [][Criterion](#criterion) | Нет | Условия (логика AND), пустой = всегда совпадает |
| `selector` | [ValuesSelector](#valuesselector) | Да | Селектор для инъекции при совпадении |

### AddonPhaseStatus

| Поле | Тип | Описание |
|------|-----|----------|
| `observedGeneration` | int64 | Последняя обработанная spec.generation |
| `ruleStatuses` | [][RuleStatus](#rulestatus) | Состояние вычисления каждого правила |
| `conditions` | []Condition | Conditions текущего состояния |

### RuleStatus

| Поле | Тип | Описание |
|------|-----|----------|
| `name` | string | Имя правила |
| `matched` | bool | Удовлетворены ли criteria |
| `message` | string | Контекст вычисления |
| `lastEvaluated` | Time | Время последнего вычисления |

### Пример

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
          jsonPath: /status/conditions/0/status
          operator: Equal
          value: "True"
      selector:
        name: tls-values
        priority: 20
        matchLabels:
          addons.in-cloud.io/addon: my-app
          addons.in-cloud.io/feature.tls: "true"
```

---

## Общие типы

### ValuesSelector

Определяет как выбирать AddonValue ресурсы.

| Поле | Тип | Обязательно | По умолчанию | Описание |
|------|-----|-------------|--------------|----------|
| `name` | string | Да | - | Идентификатор селектора |
| `priority` | int | Нет | 0 | Порядок слияния (0-100, больший перезаписывает) |
| `matchLabels` | map[string]string | Да | - | Селектор меток |

### ValueSource

Определяет внешний источник для извлечения values.

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `name` | string | Да | Идентификатор источника |
| `sourceRef` | [SourceRef](#sourceref) | Да | Ссылка на ресурс |
| `extract` | [][ExtractRule](#extractrule) | Да | Правила извлечения (мин. 1) |

### SourceRef

Ссылается на любой Kubernetes ресурс. Контроллер автоматически создаёт dynamic watch и триггерит reconcile при изменении ресурса.

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `apiVersion` | string | Да | API версия (например, "v1", "apps/v1", "cert-manager.io/v1") |
| `kind` | string | Да | Любой тип ресурса (Secret, ConfigMap, Deployment, Service, CRD и др.) |
| `name` | string | Да | Имя ресурса |
| `namespace` | string | Нет | Namespace. Обязателен для namespaced ресурсов, не нужен для cluster-scoped |

### ExtractRule

Определяет извлечение values из источника.

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `jsonPath` | string | Да | Путь для извлечения (синтаксис JSONPath) |
| `as` | string | Да | Целевой путь в объединённых values |
| `decode` | string | Нет | Декодирование ("base64" или пусто) |

### Dependency

Определяет блокирующую зависимость.

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `name` | string | Да | Имя аддона-зависимости |
| `criteria` | [][Criterion](#criterion) | Да | Условия для удовлетворения (мин. 1) |

### Criterion

Условие для вычисления по ресурсу.

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `source` | [CriterionSource](#criterionsource) | Нет | Ресурс для вычисления (по умолчанию зависимость) |
| `jsonPath` | string | Да | Путь к значению (RFC 6901 JSON Pointer) |
| `operator` | [CriterionOperator](#criterionoperator) | Да | Оператор сравнения |
| `value` | any | Нет | Ожидаемое значение (обязательно для операторов сравнения) |

### CriterionSource

Идентифицирует ресурс для вычисления.

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `apiVersion` | string | Да | API версия |
| `kind` | string | Да | Тип ресурса |
| `name` | string | Да | Имя ресурса |
| `namespace` | string | Нет | Namespace. Обязателен для namespaced ресурсов |
| `labelSelector` | LabelSelector | Нет | Выбор нескольких ресурсов |

### CriterionOperator

| Оператор | Описание | Требует Value |
|----------|----------|---------------|
| `Equal` | Значения равны | Да |
| `NotEqual` | Значения не равны | Да |
| `In` | Значение в списке | Да (массив) |
| `NotIn` | Значение не в списке | Да (массив) |
| `Exists` | Путь существует | Нет |
| `NotExists` | Путь не существует | Нет |
| `GreaterThan` | Числовое больше | Да |
| `GreaterOrEqual` | Числовое больше или равно | Да |
| `LessThan` | Числовое меньше | Да |
| `LessOrEqual` | Числовое меньше или равно | Да |
| `Matches` | Совпадение с regex | Да (строка) |

### BackendSpec

Настраивает бэкенд развёртывания.

| Поле | Тип | Обязательно | По умолчанию | Описание |
|------|-----|-------------|--------------|----------|
| `type` | string | Нет | "argocd" | Тип бэкенда |
| `namespace` | string | Да | - | Namespace бэкенда |
| `project` | string | Нет | "default" | Argo CD project |
| `syncPolicy` | [SyncPolicy](#syncpolicy) | Нет | - | Конфигурация синхронизации |
| `ignoreDifferences` | [][ResourceIgnoreDifferences](#resourceignoredifferences) | Нет | - | Правила игнорирования drift |

### SyncPolicy

| Поле | Тип | Описание |
|------|-----|----------|
| `automated` | [AutomatedSync](#automatedsync) | Настройки авто-синхронизации |
| `syncOptions` | []string | Дополнительные опции синхронизации |
| `managedNamespaceMetadata` | [ManagedNamespaceMetadata](#managednamespacemetadata) | Metadata для target namespace |

### AutomatedSync

| Поле | Тип | По умолчанию | Описание |
|------|-----|--------------|----------|
| `prune` | bool | false | Удалять ресурсы отсутствующие в Git |
| `selfHeal` | bool | false | Авто-восстановление out-of-sync ресурсов |
| `allowEmpty` | bool | false | Разрешить синхронизацию без ресурсов |

### ManagedNamespaceMetadata

Определяет labels и annotations для target namespace. Применяется при использовании `CreateNamespace=true` в syncOptions.

| Поле | Тип | Описание |
|------|-----|----------|
| `labels` | map[string]string | Labels для namespace |
| `annotations` | map[string]string | Annotations для namespace |

**Пример:**
```yaml
syncPolicy:
  syncOptions:
    - CreateNamespace=true
  managedNamespaceMetadata:
    labels:
      environment: production
      team: platform
    annotations:
      description: "Created by addon-operator"
```

### ResourceIgnoreDifferences

Определяет правила игнорирования drift для конкретных ресурсов. Полезно для полей, управляемых внешними контроллерами или mutating webhooks.

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `group` | string | Нет | API group ресурса (пусто для core API) |
| `kind` | string | Да | Тип ресурса |
| `name` | string | Нет | Имя ресурса (пусто = все ресурсы этого типа) |
| `namespace` | string | Нет | Namespace ресурса |
| `jsonPointers` | []string | Нет | JSON Pointers (RFC 6901) к игнорируемым полям |
| `jqPathExpressions` | []string | Нет | JQ выражения для полей |
| `managedFieldsManagers` | []string | Нет | Игнорировать поля этих managers |

**Пример:**
```yaml
backend:
  ignoreDifferences:
    # Игнорировать failurePolicy в webhooks (часто меняется внешними контроллерами)
    - group: admissionregistration.k8s.io
      kind: ValidatingWebhookConfiguration
      jsonPointers:
        - /webhooks/0/failurePolicy
    # Игнорировать replicas в Deployment (HPA управляет)
    - kind: Deployment
      name: my-deployment
      jqPathExpressions:
        - .spec.replicas
```

### ApplicationRef

| Поле | Тип | Описание |
|------|-----|----------|
| `name` | string | Имя Application |
| `namespace` | string | Namespace Application |

---

## Синтаксис JSONPath

Criteria используют синтаксис RFC 6901 JSON Pointer:

| Путь | Описание |
|------|----------|
| `/status/phase` | Доступ к `status.phase` |
| `/metadata/labels/app` | Доступ к `metadata.labels["app"]` |
| `/spec/replicas` | Доступ к `spec.replicas` |
| `/status/conditions/0/status` | Статус первого condition |

Специальные символы должны экранироваться:
- `~0` = `~`
- `~1` = `/`

## См. также

- [Справочник операторов Criteria](criteria-operators.md) — детальная документация операторов
- [Руководство пользователя](../user-guide/) — операционные руководства
- [Примеры](../examples/) — примеры реальных развёртываний
