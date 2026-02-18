# Условное развёртывание

Это руководство объясняет как использовать AddonPhase для условной инъекции values на основе состояния кластера.

## Обзор

AddonPhase позволяет:

- Активировать values при выполнении условий
- Реагировать на состояния других аддонов
- Включать фичи на основе ресурсов кластера

## Базовый AddonPhase

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
        name: tls-values
        priority: 20
        matchLabels:
          addons.in-cloud.io/addon: my-app
          addons.in-cloud.io/feature.tls: "true"
```

Когда cert-manager готов, TLS feature values активируются для my-app.

## Как это работает

```
┌──────────────┐     вычисляет    ┌──────────────┐
│  AddonPhase  │────────────────►│   Criteria   │
│   (rules)    │                  │   (кластер)  │
└──────┬───────┘                  └──────────────┘
       │
       │ при совпадении
       ▼
┌──────────────┐     выбирает    ┌──────────────┐
│   Selector   │────────────────►│  AddonValue  │
│ (matchLabels)│                  │  (feature)   │
└──────────────┘                  └──────────────┘
       │
       │ инъектирует в
       ▼
┌──────────────┐
│    Addon     │
│   (status)   │
└──────────────┘
```

1. AddonPhase вычисляет rules по состоянию кластера
2. Selectors совпавших rules инъектируются в статус Addon
3. Addon controller подхватывает дополнительные values

## Criteria

### Вычисление статуса Addon

```yaml
criteria:
  - source:
      apiVersion: addons.in-cloud.io/v1alpha1
      kind: Addon
      name: cert-manager
    jsonPath: $.status.conditions[0].status
    operator: Equal
    value: "True"
```

### Вычисление любого ресурса

```yaml
criteria:
  - source:
      apiVersion: v1
      kind: Secret
      name: tls-certificate
      namespace: default
    jsonPath: $.data['tls.crt']
    operator: Exists
```

### Несколько criteria (логика AND)

Все criteria должны быть выполнены:

```yaml
criteria:
  - source:
      apiVersion: addons.in-cloud.io/v1alpha1
      kind: Addon
      name: cert-manager
    jsonPath: $.status.conditions[0].status
    operator: Equal
    value: "True"
  - source:
      apiVersion: addons.in-cloud.io/v1alpha1
      kind: Addon
      name: external-dns
    jsonPath: $.status.conditions[0].status
    operator: Equal
    value: "True"
```

## Операторы

| Оператор | Описание |
|----------|----------|
| `Equal` | Значение равно ожидаемому |
| `NotEqual` | Значение не равно ожидаемому |
| `In` | Значение в списке |
| `NotIn` | Значение не в списке |
| `Exists` | Путь существует |
| `NotExists` | Путь не существует |
| `GreaterThan` | Числовое больше |
| `GreaterOrEqual` | Числовое больше или равно |
| `LessThan` | Числовое меньше |
| `LessOrEqual` | Числовое меньше или равно |
| `Matches` | Совпадение с regex |

Подробнее см. [Справочник операторов Criteria](../reference/criteria-operators.md).

## Latching (фиксация criteria)

По умолчанию, каждый criterion фиксируется при первом совпадении правила (поле `keep`, по умолчанию `true`). Зафиксированный criterion больше не перевычисляется — это означает, что если cert-manager временно станет недоступным, TLS values не будут удалены.

Criteria с `keep: false` продолжают перевычисляться каждый цикл. Можно комбинировать оба варианта в одном правиле:

```yaml
criteria:
  # Фиксируется — cert-manager может временно упасть
  - source:
      apiVersion: addons.in-cloud.io/v1alpha1
      kind: Addon
      name: cert-manager
    jsonPath: $.status.conditions[?@.type=='Ready'].status
    operator: Equal
    value: "True"
    # keep: true (по умолчанию)

  # Перевычисляется — правило деактивируется при масштабировании вниз
  - source:
      apiVersion: apps/v1
      kind: Deployment
      name: my-app
      namespace: default
    jsonPath: $.spec.replicas
    operator: GreaterOrEqual
    value: 3
    keep: false
```

## Примеры

### Включение фичи при готовности зависимости

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonPhase
metadata:
  name: istio-app
spec:
  rules:
    - name: enable-mtls
      criteria:
        - source:
            apiVersion: addons.in-cloud.io/v1alpha1
            kind: Addon
            name: istio
          jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
      selector:
        name: mtls-values
        priority: 30
        matchLabels:
          addons.in-cloud.io/addon: my-app
          addons.in-cloud.io/feature.mtls: "true"
```

### Включение по метке окружения

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonPhase
metadata:
  name: prometheus-features
spec:
  rules:
    - name: high-availability
      criteria:
        - source:
            apiVersion: v1
            kind: ConfigMap
            name: cluster-info
            namespace: kube-system
          jsonPath: $.data.environment
          operator: Equal
          value: "production"
      selector:
        name: ha-values
        priority: 25
        matchLabels:
          addons.in-cloud.io/addon: prometheus
          addons.in-cloud.io/feature.ha: "true"
```

### Многоэтапное развёртывание

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonPhase
metadata:
  name: app-stages
spec:
  rules:
    - name: stage-1-networking
      criteria:
        - source:
            apiVersion: addons.in-cloud.io/v1alpha1
            kind: Addon
            name: cilium
          jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
      selector:
        name: networking-stage
        priority: 10
        matchLabels:
          addons.in-cloud.io/addon: my-app
          addons.in-cloud.io/stage: networking

    - name: stage-2-security
      criteria:
        - source:
            apiVersion: addons.in-cloud.io/v1alpha1
            kind: Addon
            name: cert-manager
          jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
        - source:
            apiVersion: addons.in-cloud.io/v1alpha1
            kind: Addon
            name: external-secrets
          jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
      selector:
        name: security-stage
        priority: 20
        matchLabels:
          addons.in-cloud.io/addon: my-app
          addons.in-cloud.io/stage: security
```

## Проверка статуса Phase

```bash
kubectl get addonphase my-app -o yaml
```

Статус показывает результаты вычисления rules:

```yaml
status:
  ruleStatuses:
    - name: enable-tls
      matched: true
      message: "All conditions satisfied"
      lastEvaluated: "2024-01-15T10:00:00Z"
```

## Отладка

Проверка почему rule не срабатывает:

```bash
# Просмотр статуса rule
kubectl get addonphase my-app -o jsonpath='{.status.ruleStatuses}'

# Проверка source ресурса
kubectl get addon cert-manager -o jsonpath='{.status.conditions[0]}'
```

## Следующие шаги

- [Фиксация правил (Latching)](rule-latching.md) — защита от каскадных сбоев при обновлении зависимостей
- [Зависимости](dependencies.md) — блокировка развёртывания до готовности зависимостей
- [Справочник операторов Criteria](../reference/criteria-operators.md) — все доступные операторы
