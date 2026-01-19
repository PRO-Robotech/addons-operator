# AddonPhase

Ресурс AddonPhase обеспечивает условную активацию фич на основе состояния кластера.

## Обзор

AddonPhase:
- Имеет связь **1:1** с Addon (одинаковое имя, оба ресурса cluster-scoped)
- Определяет **правила** с criteria, которые вычисляются по состоянию кластера
- Когда правила совпадают, их **селекторы** добавляются в Addon
- Включает динамическую активацию фич без ручного вмешательства

## Сценарии использования

- Включение TLS фич когда cert-manager готов
- Активация экспортёров мониторинга когда Prometheus развёрнут
- Настройка сетевых политик на основе состояния CNI
- Включение фич ingress когда ingress controller готов

## Поля Spec

### Rules

Каждое правило содержит:

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Уникальное имя правила |
| `criteria` | Нет | Условия, которые должны быть выполнены (пустой = всегда активно) |
| `selector` | Да | ValuesSelector для добавления при совпадении правила |

### Criteria

Каждый criterion вычисляет условие:

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `source` | Нет | Внешний ресурс для вычисления (по умолчанию: целевой Addon) |
| `jsonPath` | Да | Путь к значению (например, `/status/conditions/0/status`) |
| `operator` | Да | Оператор сравнения |
| `value` | Нет | Ожидаемое значение (обязательно для большинства операторов) |

### Source

Когда `source` указан, criteria вычисляются для внешних ресурсов:

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `apiVersion` | Да | API версия ресурса |
| `kind` | Да | Тип ресурса |
| `name` | Да | Имя ресурса |
| `namespace` | Нет | Namespace ресурса. Обязателен для namespaced ресурсов |

## Пример

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonPhase
metadata:
  name: cilium  # Должно совпадать с именем Addon (cluster-scoped, без namespace)
spec:
  rules:
    # Всегда активное правило (нет criteria)
    - name: hubble
      selector:
        name: hubble
        priority: 10
        matchLabels:
          addons.in-cloud.io/addon: cilium
          addons.in-cloud.io/feature.hubble: "true"

    # Условное правило — активируется когда cert-manager Ready
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

## Status

### Rule Statuses

```yaml
status:
  ruleStatuses:
    - name: hubble
      matched: true
      message: "No conditions"
    - name: certificates
      matched: false
      message: "Criterion failed: cert-manager conditions/0/status != True"
```

## Операторы

| Оператор | Описание | Пример |
|----------|----------|--------|
| `Equal` | Точное совпадение | `status == "True"` |
| `NotEqual` | Не равно | `status != "False"` |
| `In` | Значение в списке | `env in ["prod", "staging"]` |
| `NotIn` | Значение не в списке | `env not in ["dev"]` |
| `Exists` | Путь существует | `annotations.foo exists` |
| `NotExists` | Путь не существует | `status.error not exists` |
| `GreaterThan` | Числовое больше | `replicas > 1` |
| `GreaterOrEqual` | Числовое >= | `replicas >= 2` |
| `LessThan` | Числовое меньше | `replicas < 10` |
| `LessOrEqual` | Числовое <= | `replicas <= 5` |
| `Matches` | Совпадение с regex | `name matches "^prod-.*"` |

## Синтаксис JSONPath

Criteria используют упрощённую нотацию JSONPath:

```
/status/conditions/0/status  → status.conditions[0].status
/metadata/annotations        → metadata.annotations
/spec/replicas               → spec.replicas
```

**Важно:** Используйте префикс `/` для JSONPath в criteria (не `.`).

## Как это работает

```
┌─────────────┐     наблюдает    ┌─────────────┐
│ AddonPhase  │─────────────────▶│   Sources   │
│   (rules)   │                  │(Addons,etc) │
└─────┬───────┘                  └─────────────┘
      │
      │ вычисляет criteria
      │ добавляет совпавшие селекторы
      ▼
┌─────────────┐
│   Addon     │
│ (phaseVals) │
└─────────────┘
```

1. AddonPhase наблюдает за исходными ресурсами на изменения
2. При изменении источников criteria перевычисляются
3. Совпавшие правила добавляют свои селекторы в `status.phaseValuesSelector` Addon
4. Контроллер Addon включает phaseValuesSelector в выбор values
5. Application обновляется с новыми values

## Очистка

При удалении AddonPhase:
- Finalizer обеспечивает очистку
- Все phaseValuesSelectors удаляются из Addon
- Addon reconcile выполняется без этих селекторов

## Лучшие практики

### 1. Именуйте AddonPhase так же как Addon

```yaml
metadata:
  name: cilium  # Совпадает с именем Addon
```

### 2. Используйте метки фич

```yaml
selector:
  name: tls-feature
  priority: 20
  matchLabels:
    addons.in-cloud.io/addon: cilium
    addons.in-cloud.io/feature.tls: "true"
```

### 3. Устанавливайте подходящие приоритеты

- Базовые фичи: приоритет 10-30
- Опциональные фичи: приоритет 40-60
- Переопределяющие фичи: приоритет 70-90

### 4. Обрабатывайте отсутствующие источники

Правила с несуществующими источниками не совпадут. Это ожидаемое поведение — правило активируется, когда источник создан и готов.

## Связанные ресурсы

- [Addon](addon.md) — цель phaseValuesSelector
- [AddonValue](addon-value.md) — values, выбираемые правилами
- [Операторы Criteria](../reference/criteria-operators.md) — полный справочник операторов
