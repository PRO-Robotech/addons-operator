# AddonValue

Ресурс AddonValue хранит конфигурационные values для Addon.

## Обзор

AddonValue:
- Хранит Helm values в структурированном формате
- Использует **метки** для выбора ресурсами Addon
- Поддерживает **рендеринг шаблонов** с Go templates
- Может использоваться несколькими Addon

## Поля Spec

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `values` | Да | Helm values в формате JSON/YAML (runtime.RawExtension) |

## Метки

AddonValue выбираются Addon с помощью селекторов меток. Типичные паттерны меток:

| Метка | Назначение |
|-------|------------|
| `addons.in-cloud.io/addon` | Связь с конкретным Addon |
| `addons.in-cloud.io/layer` | Слой приоритета (defaults, custom, immutable) |
| `addons.in-cloud.io/feature.<name>` | Флаги фич для AddonPhase |

> **Важно:** При матчинге AddonValue контроллер учитывает **только** метки с префиксом `addons.in-cloud.io/`. Все остальные метки (например, `kubernetes.io/*`, `app.kubernetes.io/*` или метки без префикса) игнорируются. Это означает, что метка `layer: defaults` без префикса **не будет работать** — необходимо использовать `addons.in-cloud.io/layer: defaults`.

## Примеры

### Базовый AddonValue

```yaml
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
      name: production
    hubble:
      enabled: true
      relay:
        enabled: true
    operator:
      replicas: 2
```

### AddonValue с шаблонами

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: cilium-templated
  labels:
    addons.in-cloud.io/addon: cilium
    addons.in-cloud.io/layer: custom
spec:
  values:
    cluster:
      name: "{{ .Variables.cluster_name }}"
    hubble:
      ui:
        ingress:
          hosts:
            - "hubble.{{ .Variables.domain }}"
```

Когда Addon определяет переменные:

```yaml
spec:
  variables:
    cluster_name: production
    domain: example.com
```

Отрендеренные values становятся:

```yaml
cluster:
  name: production
hubble:
  ui:
    ingress:
      hosts:
        - hubble.example.com
```

## Синтаксис шаблонов

AddonValue поддерживает Go templates со следующим контекстом:

| Переменная | Описание |
|------------|----------|
| `.Variables` | Переменные из spec Addon |
| `.Values` | Values из valuesSources |

### Функции шаблонов

Доступные функции:

```yaml
# Значение по умолчанию, если переменная не задана
cluster: "{{ .Variables.cluster_name | default \"default-cluster\" }}"

# Обернуть в кавычки
name: "{{ .Variables.name | quote }}"

# Base64 кодирование/декодирование
secret: "{{ .Values.password | b64enc }}"

# Условия
{{ if .Variables.enable_tls }}
tls:
  enabled: true
{{ end }}
```

## Процесс выбора

1. `valuesSelectors` Addon определяют селекторы меток
2. Контроллер находит все соответствующие AddonValue
3. Values сортируются по приоритету селектора
4. Values глубоко объединяются (больший приоритет выигрывает)

### Пример выбора

Spec Addon:
```yaml
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
```

Соответствующие AddonValue:
- `cilium-defaults` (приоритет 0): `{ replicas: 1, memory: "128Mi" }`
- `cilium-custom` (приоритет 50): `{ replicas: 3 }`

Результат:
```yaml
replicas: 3       # Из приоритета 50
memory: "128Mi"   # Из приоритета 0
```

## Лучшие практики

### 1. Используйте согласованные метки

```yaml
metadata:
  labels:
    addons.in-cloud.io/addon: <addon-name>
    addons.in-cloud.io/layer: <defaults|custom|immutable>
```

### 2. Разделяйте ответственности

- **defaults**: Базовая конфигурация
- **custom**: Переопределения для окружения
- **immutable**: Настройки безопасности/compliance (наивысший приоритет)

### 3. Используйте шаблоны для динамических значений

Вместо хардкода:
```yaml
cluster: production-us-east-1
```

Используйте шаблоны:
```yaml
cluster: "{{ .Variables.cluster_name }}-{{ .Variables.region }}"
```

### 4. Документируйте values

Добавляйте комментарии, объясняющие конфигурацию:
```yaml
spec:
  values:
    # Количество реплик оператора для HA
    operator:
      replicas: 2
```

## Связанные ресурсы

- [Addon](addon.md) — выбор values
- [valuesSources](values-sources.md) — извлечённые values в шаблонах
