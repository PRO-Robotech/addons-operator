# valuesSources

valuesSources позволяет извлекать данные из **любых** Kubernetes ресурсов (Secrets, ConfigMaps, Deployments, Services, CRDs и др.) и использовать их в шаблонах values.

## Обзор

valuesSources:
- Извлекает данные из **любых Kubernetes ресурсов** (Secrets, ConfigMaps, Deployments, Services, CRDs и др.)
- Поддерживает **декодирование base64** для данных Secret
- Делает извлечённые значения доступными как `.Values` в шаблонах
- Автоматически **наблюдает** за исходными ресурсами на изменения

## Поля Spec

### ValueSource

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `name` | Да | Уникальное имя этого источника |
| `sourceRef` | Да | Ссылка на исходный ресурс |
| `extract` | Да | Список правил извлечения |

### SourceRef

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `apiVersion` | Да | API версия ресурса (`v1`, `apps/v1`, `networking.k8s.io/v1`, CRD apiVersion и др.) |
| `kind` | Да | Тип ресурса (любой: `Secret`, `ConfigMap`, `Deployment`, `Service`, CRD и др.) |
| `name` | Да | Имя ресурса |
| `namespace` | Нет | Namespace ресурса. Обязателен для namespaced ресурсов (Secret, ConfigMap). Не нужен для cluster-scoped (ClusterIssuer, Node) |

### ExtractRule

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `jsonPath` | Да | Путь к значению (синтаксис `.data.key`) |
| `as` | Да | Целевой путь в `.Values` |
| `decode` | Нет | Метод декодирования (`base64` для Secrets) |

## Примеры

### Извлечение из Secret

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: my-app
spec:
  chart: my-chart
  repoURL: https://charts.example.com
  version: "1.0.0"
  targetCluster: in-cluster
  targetNamespace: default
  backend:
    type: argocd
    namespace: argocd

  valuesSources:
    - name: database
      sourceRef:
        apiVersion: v1
        kind: Secret
        name: database-credentials
        namespace: default
      extract:
        - jsonPath: .data.username
          as: db.username
          decode: base64
        - jsonPath: .data.password
          as: db.password
          decode: base64

  valuesSelectors:
    - name: default
      matchLabels:
        addons.in-cloud.io/addon: my-app
```

### Использование в шаблоне AddonValue

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: my-app-values
  labels:
    addons.in-cloud.io/addon: my-app
spec:
  values:
    database:
      host: postgres.default.svc
      port: 5432
      username: "{{ .Values.db.username }}"
      password: "{{ .Values.db.password }}"
```

### Извлечение из ConfigMap

```yaml
valuesSources:
  - name: config
    sourceRef:
      apiVersion: v1
      kind: ConfigMap
      name: app-config
    extract:
      - jsonPath: .data.settings
        as: config.settings
        # Декодирование не нужно для ConfigMap
```

### Извлечение из других ресурсов

valuesSources поддерживает извлечение данных из любого Kubernetes ресурса:

#### Из Deployment

```yaml
valuesSources:
  - name: app-info
    sourceRef:
      apiVersion: apps/v1
      kind: Deployment
      name: my-app
    extract:
      - jsonPath: .spec.replicas
        as: app.replicas
      - jsonPath: .spec.template.spec.containers[0].image
        as: app.image
```

#### Из Service

```yaml
valuesSources:
  - name: service-info
    sourceRef:
      apiVersion: v1
      kind: Service
      name: my-service
    extract:
      - jsonPath: .spec.clusterIP
        as: service.ip
      - jsonPath: .spec.ports[0].port
        as: service.port
```

#### Из Custom Resource (CRD)

```yaml
valuesSources:
  - name: addon-status
    sourceRef:
      apiVersion: addons.in-cloud.io/v1alpha1
      kind: Addon
      name: cert-manager
    extract:
      - jsonPath: .status.conditions[0].status
        as: certmanager.ready
```

> **Примечание:** `decode: base64` имеет смысл только для данных из Secret. Для других ресурсов декодирование обычно не требуется.

## Синтаксис JSONPath

valuesSources использует нотацию JSONPath с префиксом `.`:

### Примеры для Secret/ConfigMap

```
.data.username       → data["username"]
.data["ca.crt"]      → data["ca.crt"] (для ключей с точками)
.stringData.config   → stringData["config"]
```

### Примеры для других ресурсов

```
.spec.replicas                           → количество реплик (Deployment)
.spec.template.spec.containers[0].image  → образ первого контейнера
.spec.clusterIP                          → ClusterIP сервиса (Service)
.spec.ports[0].port                      → первый порт (Service)
.status.conditions[0].status             → статус первого условия
.status.phase                            → фаза ресурса
.metadata.labels["app.kubernetes.io/name"] → значение label
```

**Важно:** Используйте префикс `.` для JSONPath в valuesSources (не `/`).

### Ключи со специальными символами

Для ключей, содержащих точки или специальные символы, используйте скобочную нотацию:

```yaml
extract:
  - jsonPath: .data["ca.crt"]
    as: tls.ca
    decode: base64
```

## Контекст шаблонов

Извлечённые значения доступны в шаблонах как `.Values`:

| Путь | Значение |
|------|----------|
| `.Values.db.username` | Извлечённое имя пользователя БД |
| `.Values.db.password` | Извлечённый пароль БД |
| `.Values.config.settings` | Извлечённые настройки конфигурации |

## Обработка ошибок

### Источник не найден

Если исходный ресурс не существует:
- Статус Addon: `ValuesResolved=False`
- Reason: `ValueSourceError`
- Application **не** создаётся

Когда источник создан, Addon автоматически reconcile и восстанавливается.

### Путь не найден

Если JSONPath не существует в источнике:
- Статус Addon: `ValuesResolved=False`
- Reason: `ValueSourceError`
- Проверьте синтаксис JSONPath

## Наблюдение за источниками

Контроллер автоматически наблюдает за исходными ресурсами:
- При изменении ресурса (Secret, ConfigMap, Deployment и др.) затронутые Addon выполняют reconcile
- Values перезвлекаются и шаблоны перерендериваются
- Application обновляется с новыми values

## Полный пример

### Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: tls-certificates
type: Opaque
data:
  ca.crt: LS0tLS1CRUdJTi... # закодировано в base64
  tls.crt: LS0tLS1CRUdJTi...
  tls.key: LS0tLS1CRUdJTi...
```

### Addon с valuesSources

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: nginx-ingress
spec:
  chart: ingress-nginx
  repoURL: https://kubernetes.github.io/ingress-nginx
  version: "4.8.0"
  targetCluster: in-cluster
  targetNamespace: ingress-nginx
  backend:
    type: argocd
    namespace: argocd

  valuesSources:
    - name: tls
      sourceRef:
        apiVersion: v1
        kind: Secret
        name: tls-certificates
      extract:
        - jsonPath: .data["ca.crt"]
          as: tls.ca
          decode: base64
        - jsonPath: .data["tls.crt"]
          as: tls.cert
          decode: base64
        - jsonPath: .data["tls.key"]
          as: tls.key
          decode: base64

  valuesSelectors:
    - name: default
      matchLabels:
        addons.in-cloud.io/addon: nginx-ingress
```

### AddonValue с извлечёнными значениями

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: nginx-ingress-values
  labels:
    addons.in-cloud.io/addon: nginx-ingress
spec:
  values:
    controller:
      extraArgs:
        default-ssl-certificate: ingress-nginx/default-tls
      extraVolumes:
        - name: tls
          secret:
            secretName: ingress-tls
      config:
        ssl-ciphers: "HIGH:!aNULL:!MD5"
    defaultBackend:
      enabled: true
    tcp:
      configMapNamespace: ingress-nginx
```

## Лучшие практики

### 1. Используйте понятные имена источников

```yaml
valuesSources:
  - name: database-credentials  # Ясное назначение
  - name: tls-certificates      # Ясное назначение
```

### 2. Всегда декодируйте данные Secret

```yaml
extract:
  - jsonPath: .data.password
    as: auth.password
    decode: base64  # Обязательно для данных Secret
```

### 3. Обрабатывайте отсутствующие источники корректно

Создайте исходные ресурсы до Addon, или будьте готовы к тому, что Addon останется в состоянии ошибки пока источник не будет создан.

### 4. Используйте декодирование только для Secret

```yaml
# Для Secret — используйте decode: base64
extract:
  - jsonPath: .data.password
    decode: base64

# Для других ресурсов — декодирование не нужно
extract:
  - jsonPath: .spec.replicas
    as: app.replicas
    # decode не указываем
```

## Связанные ресурсы

- [Addon](addon.md) — конфигурация valuesSources
- [AddonValue](addon-value.md) — синтаксис шаблонов
