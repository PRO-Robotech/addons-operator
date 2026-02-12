# Справочник операторов Criteria

Этот документ описывает все операторы, доступные для criteria в `initDependencies` и правилах AddonPhase.

## Операторы сравнения

### Equal

Проверяет, что значение по JSONPath равно ожидаемому.

```yaml
criteria:
  - jsonPath: $.status.conditions[?@.type=='Ready'].status
    operator: Equal
    value: "True"
```

### NotEqual

Проверяет, что значение по JSONPath не равно ожидаемому.

```yaml
criteria:
  - jsonPath: $.status.conditions[?@.type=='Ready'].status
    operator: NotEqual
    value: "False"
```

## Операторы множеств

### In

Проверяет, что значение по JSONPath входит в предоставленный список.

```yaml
criteria:
  - jsonPath: $.metadata.labels.environment
    operator: In
    value: ["production", "staging"]
```

### NotIn

Проверяет, что значение по JSONPath не входит в предоставленный список.

```yaml
criteria:
  - jsonPath: $.metadata.labels.environment
    operator: NotIn
    value: ["development", "test"]
```

## Операторы существования

### Exists

Проверяет, что путь существует в ресурсе (значение может быть любым, включая null).

```yaml
criteria:
  - jsonPath: $.status.applicationRef
    operator: Exists
    # Значение не требуется
```

### NotExists

Проверяет, что путь не существует в ресурсе.

```yaml
criteria:
  - jsonPath: $.status.error
    operator: NotExists
    # Значение не требуется
```

## Числовые операторы

### GreaterThan

Проверяет, что числовое значение больше ожидаемого.

```yaml
criteria:
  - jsonPath: $.spec.replicas
    operator: GreaterThan
    value: 1
```

### GreaterOrEqual

Проверяет, что числовое значение больше или равно ожидаемому.

```yaml
criteria:
  - jsonPath: $.spec.replicas
    operator: GreaterOrEqual
    value: 2
```

### LessThan

Проверяет, что числовое значение меньше ожидаемого.

```yaml
criteria:
  - jsonPath: $.spec.replicas
    operator: LessThan
    value: 10
```

### LessOrEqual

Проверяет, что числовое значение меньше или равно ожидаемому.

```yaml
criteria:
  - jsonPath: $.spec.replicas
    operator: LessOrEqual
    value: 5
```

## Паттерны

### Matches

Проверяет, что строковое значение соответствует регулярному выражению.

```yaml
criteria:
  - jsonPath: $.metadata.name
    operator: Matches
    value: "^prod-.*"
```

## Поведение при отсутствии пути

| Оператор | Результат когда путь не найден |
|----------|-------------------------------|
| Equal | `false` |
| NotEqual | `true` |
| In | `false` |
| NotIn | `true` |
| Exists | `false` |
| NotExists | `true` |
| GreaterThan | `false` |
| GreaterOrEqual | `false` |
| LessThan | `false` |
| LessOrEqual | `false` |
| Matches | `false` |

## Типы значений

### Строковые значения

```yaml
value: "True"
```

### Числовые значения

```yaml
value: 3
```

### Булевы значения

```yaml
value: true
```

### Списки (для In/NotIn)

```yaml
value: ["production", "staging", "qa"]
```

### Null значения

```yaml
value: null
```

## Синтаксис JSONPath

Criteria используют RFC 9535 JSONPath синтаксис (`github.com/theory/jsonpath`). Путь начинается с `$`:

```
$.status.phase                                   → status.phase
$.status.conditions[0].status                    → первый condition
$.status.conditions[?@.type=='Ready'].status     → filter по type
$.metadata.labels['app.kubernetes.io/name']      → ключ с точками (bracket notation)
$.spec.template.spec.containers[0].name          → вложенный массив
```

## Примеры

### Множественные Criteria (логика AND)

Все criteria должны быть выполнены:

```yaml
criteria:
  - jsonPath: $.status.conditions[?@.type=='Ready'].status
    operator: Equal
    value: "True"
  - jsonPath: $.spec.replicas
    operator: GreaterOrEqual
    value: 2
```

### Внешний источник

Вычисление criteria по другому ресурсу:

```yaml
criteria:
  - source:
      apiVersion: addons.in-cloud.io/v1alpha1
      kind: Addon
      name: cert-manager
    jsonPath: $.status.conditions[?@.type=='Ready'].status
    operator: Equal
    value: "True"
```

### Проверка Argo CD Application

Проверка статуса Argo CD Application:

```yaml
criteria:
  - source:
      apiVersion: argoproj.io/v1alpha1
      kind: Application
      name: my-app
      namespace: argocd
    jsonPath: $.status.health.status
    operator: Equal
    value: "Healthy"
```

### Проверка Ready condition

Типичный паттерн для проверки готовности Addon:

```yaml
criteria:
  - source:
      apiVersion: addons.in-cloud.io/v1alpha1
      kind: Addon
      name: dependency-addon
    jsonPath: $.status.conditions[?@.type=='Ready'].status
    operator: Equal
    value: "True"
```

## См. также

- [AddonPhase](../concepts/addon-phase.md) — условная активация фич
- [Зависимости](../user-guide/dependencies.md) — управление зависимостями
