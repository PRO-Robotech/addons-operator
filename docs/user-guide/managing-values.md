# Управление values

Это руководство объясняет как настраивать values аддонов с помощью AddonValue ресурсов и источников значений.

## AddonValue ресурсы

AddonValue хранит конфигурацию, которая может выбираться несколькими Addon'ами:

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
    grafana:
      enabled: true
      adminPassword: admin
```

## Выбор values

Addon'ы выбирают values с помощью label селекторов:

```yaml
spec:
  valuesSelectors:
    - name: base
      priority: 0
      matchLabels:
        addons.in-cloud.io/addon: prometheus
        addons.in-cloud.io/layer: base
```

## Слияние по приоритету

Несколько AddonValue объединяются по приоритету (больший приоритет перезаписывает меньший):

```yaml
# Приоритет 0 — базовые values
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: prometheus-base
  labels:
    addons.in-cloud.io/addon: prometheus
    addons.in-cloud.io/layer: base
spec:
  values:
    replicas: 1
    resources:
      requests:
        memory: 256Mi

---
# Приоритет 10 — production values (перезаписывает base)
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: prometheus-production
  labels:
    addons.in-cloud.io/addon: prometheus
    addons.in-cloud.io/environment: production
spec:
  values:
    replicas: 3
    resources:
      requests:
        memory: 1Gi
```

Конфигурация Addon:

```yaml
spec:
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
```

Результат (объединённый):

```yaml
replicas: 3           # Из production (приоритет 10)
resources:
  requests:
    memory: 1Gi       # Из production (приоритет 10)
```

## Источники values

Загрузка values напрямую из любых Kubernetes ресурсов — Secrets, ConfigMaps, Deployments, Services, CRDs и др. Контроллер автоматически отслеживает изменения в исходных ресурсах.

### Из Secret

```yaml
spec:
  valuesSources:
    - name: credentials
      sourceRef:
        apiVersion: v1
        kind: Secret
        name: prometheus-credentials
      extract:
        - jsonPath: .data.password
          as: grafana.adminPassword
          decode: base64
```

Формат Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: prometheus-credentials
data:
  password: c3VwZXJzZWNyZXQ=  # base64: supersecret
```

### Из ConfigMap

```yaml
spec:
  valuesSources:
    - name: config
      sourceRef:
        apiVersion: v1
        kind: ConfigMap
        name: prometheus-config
      extract:
        - jsonPath: .data.settings
          as: config.settings
```

### Из других ресурсов

valuesSources поддерживает любые Kubernetes ресурсы:

```yaml
spec:
  valuesSources:
    # Из Deployment
    - name: app-info
      sourceRef:
        apiVersion: apps/v1
        kind: Deployment
        name: my-app
      extract:
        - jsonPath: .spec.replicas
          as: app.replicas

    # Из Custom Resource
    - name: addon-status
      sourceRef:
        apiVersion: addons.in-cloud.io/v1alpha1
        kind: Addon
        name: cert-manager
      extract:
        - jsonPath: .status.conditions[0].status
          as: certmanager.ready
```

Подробнее см. [valuesSources](../concepts/values-sources.md).

## Go шаблоны

AddonValue поддерживает Go шаблоны с переменными:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: prometheus-cluster
  labels:
    addons.in-cloud.io/addon: prometheus
spec:
  values:
    cluster:
      name: "{{ .Variables.cluster_name }}"
    prometheus:
      externalLabels:
        cluster: "{{ .Variables.cluster_name }}"
        environment: "{{ .Variables.environment }}"
```

Определение переменных в Addon:

```yaml
spec:
  variables:
    cluster_name: production-cluster
    environment: production
```

## Проверка агрегированных values

Просмотр итоговых объединённых values (хранятся в Argo CD Application):

```bash
kubectl get application -n argocd prometheus -o jsonpath='{.spec.source.helm.values}' | yq
```

## Организация values

Рекомендуемая схема меток:

| Метка | Назначение | Пример |
|-------|------------|--------|
| `addons.in-cloud.io/addon` | Связь с аддоном | `prometheus` |
| `addons.in-cloud.io/layer` | Слой values | `base`, `security`, `monitoring` |
| `addons.in-cloud.io/environment` | Окружение | `production`, `staging` |
| `addons.in-cloud.io/cluster` | Специфично для кластера | `us-west-1`, `eu-central-1` |

## Примеры

### Паттерн Base + Environment

```yaml
# Базовые values (приоритет 0)
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: app-base
  labels:
    addons.in-cloud.io/addon: my-app
    addons.in-cloud.io/layer: base
spec:
  values:
    image:
      repository: myapp
      tag: latest

---
# Staging окружение (приоритет 10)
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: app-staging
  labels:
    addons.in-cloud.io/addon: my-app
    addons.in-cloud.io/environment: staging
spec:
  values:
    replicas: 1
    resources:
      requests:
        cpu: 100m

---
# Production окружение (приоритет 10)
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: app-production
  labels:
    addons.in-cloud.io/addon: my-app
    addons.in-cloud.io/environment: production
spec:
  values:
    replicas: 3
    resources:
      requests:
        cpu: 500m
```

### Паттерн Feature Flags

```yaml
# TLS feature values
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: app-tls
  labels:
    addons.in-cloud.io/addon: my-app
    addons.in-cloud.io/feature.tls: "true"
spec:
  values:
    tls:
      enabled: true
      secretName: app-tls-secret
```

## Следующие шаги

- [Условное развёртывание](conditional-deployment.md) — условная активация values
- [Зависимости](dependencies.md) — ожидание зависимостей перед развёртыванием
