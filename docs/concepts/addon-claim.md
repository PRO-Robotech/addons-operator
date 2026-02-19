# AddonClaim

Ресурс AddonClaim представляет запрос на развёртывание аддона в удалённом infra-кластере.

## Обзор

AddonClaim:
- Определяет **что** развернуть (имя аддона, версия, шаблон)
- Определяет **где** развернуть (целевой кластер через kubeconfig Secret)
- Определяет **как** сконфигурировать (values или valuesString)
- Создаёт и управляет Addon и AddonValue в удалённом кластере
- Зеркалирует статус удалённого Addon обратно в system-кластер

### Мультикластерная архитектура

```
┌─────────────────────────────┐         ┌─────────────────────────────┐
│       System Cluster        │         │       Infra Cluster         │
│                             │         │                             │
│  ┌───────────┐              │         │              ┌───────────┐  │
│  │ AddonClaim│──┐           │         │           ┌──│   Addon   │  │
│  └───────────┘  │           │         │           │  └───────────┘  │
│                 ▼           │         │           │                 │
│  ┌──────────────────┐       │  kubeconfig  ┌─────▼──────┐          │
│  │   AddonClaim     │───────┼────────►│   remote    │          │
│  │   Controller     │       │         │   apply     │          │
│  └──────────────────┘       │         └─────┬──────┘          │
│         │                   │         │     │                  │
│  ┌──────▼──────┐            │         │  ┌──▼──────────┐       │
│  │ AddonTemplate│            │         │  │ AddonValue  │       │
│  └─────────────┘            │         │  └─────────────┘       │
└─────────────────────────────┘         └─────────────────────────────┘
```

AddonClaim — **namespaced** ресурс. Контроллер работает в system-кластере, а Addon и AddonValue создаются в infra-кластере.

## Поля Spec

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `name` | string | Да | Имя аддона. Используется как имя Addon в infra-кластере |
| `version` | string | Да | Версия аддона |
| `cluster` | string | Да | Имя целевого клиентского кластера (используется в рендеринге шаблона) |
| `credentialRef` | [CredentialRef](#credentialref) | Да | Ссылка на Secret с kubeconfig infra-кластера |
| `templateRef` | [TemplateRef](#templateref) | Да | Ссылка на AddonTemplate для рендеринга |
| `values` | object | Нет | Helm values как JSON объект |
| `valuesString` | string | Нет | Helm values как YAML строка (может содержать Go templates) |
| `dependency` | bool | Нет | Пометить аддон как зависимость (аннотация `dependency.addons.in-cloud.io/enabled`) |

`values` и `valuesString` взаимоисключающие — можно указать только одно из двух.

### CredentialRef

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `name` | string | Да | Имя Secret с kubeconfig |

### TemplateRef

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `name` | string | Да | Имя AddonTemplate (cluster-scoped) |

## Status

### Conditions

| Тип | Значение |
|-----|----------|
| `Ready` | AddonClaim полностью reconciled, удалённый Addon готов |
| `Progressing` | Выполняется reconciliation или ожидание готовности |
| `Degraded` | Произошла ошибка |
| `TemplateRendered` | AddonTemplate успешно отрендерен в Addon |
| `RemoteConnected` | Соединение с infra-кластером установлено |
| `AddonSynced` | Addon и AddonValue синхронизированы в infra-кластер |

### Поля Status

| Поле | Тип | Описание |
|------|-----|----------|
| `ready` | bool | Удалённый Addon в состоянии Ready |
| `deployed` | bool | Удалённый Addon был развёрнут хотя бы один раз |
| `remoteAddonStatus` | [RemoteAddonStatus](#remoteaddonstatus) | Зеркало статуса Addon из infra-кластера |
| `observedGeneration` | int64 | Последнее обработанное поколение spec |
| `conditions` | []Condition | Текущее состояние AddonClaim |

### RemoteAddonStatus

| Поле | Тип | Описание |
|------|-----|----------|
| `deployed` | bool | Addon в infra-кластере был развёрнут |
| `conditions` | []Condition | Conditions Addon из infra-кластера |

```yaml
status:
  ready: true
  deployed: true
  observedGeneration: 1
  remoteAddonStatus:
    deployed: true
    conditions:
      - type: Ready
        status: "True"
        reason: FullyReconciled
      - type: Synced
        status: "True"
        reason: Synced
  conditions:
    - type: Ready
      status: "True"
      reason: FullyReconciled
      message: "Remote Addon is ready"
    - type: Progressing
      status: "False"
      reason: Complete
    - type: Degraded
      status: "False"
      reason: Healthy
    - type: TemplateRendered
      status: "True"
      reason: Rendered
    - type: RemoteConnected
      status: "True"
      reason: Connected
    - type: AddonSynced
      status: "True"
      reason: Synced
```

## Примеры

### Минимальный

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonClaim
metadata:
  name: cilium
  namespace: tenant-a
spec:
  name: cilium
  version: v1.17.4
  cluster: client-cluster-01
  credentialRef:
    name: infra-kubeconfig
  templateRef:
    name: cilium-v1.17.4
```

### С valuesString

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonClaim
metadata:
  name: monitoring
  namespace: tenant-a
spec:
  name: monitoring
  version: "1.0.0"
  cluster: client-cluster-01
  credentialRef:
    name: infra-kubeconfig
  templateRef:
    name: monitoring-v1
  valuesString: |
    prometheus:
      replicas: 2
    grafana:
      enabled: true
```

### С dependency

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonClaim
metadata:
  name: cilium
  namespace: tenant-a
spec:
  name: cilium
  version: v1.17.4
  cluster: client-cluster-01
  credentialRef:
    name: infra-kubeconfig
  templateRef:
    name: cilium-v1.17.4
  dependency: true
```

## Credential Secret

Secret должен находиться в том же namespace, что и AddonClaim, и содержать ключ `value` с данными kubeconfig:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: infra-kubeconfig
  namespace: tenant-a
type: Opaque
data:
  value: <base64-encoded kubeconfig>
```

Контроллер использует kubeconfig для создания client'а к infra-кластеру. Клиенты кэшируются и пересоздаются при изменении `resourceVersion` Secret.

## Жизненный цикл

1. Пользователь создаёт AddonClaim
2. Контроллер рендерит AddonTemplate → Addon YAML
3. Контроллер подключается к infra-кластеру через kubeconfig
4. Контроллер создаёт/обновляет AddonValue и Addon в infra-кластере
5. Контроллер периодически опрашивает статус удалённого Addon
6. При удалении AddonClaim — удаляет Addon и AddonValue из infra-кластера

## Связанные ресурсы

- [AddonTemplate](addon-template.md) — шаблоны для генерации Addon
- [Addon](addon.md) — ресурс, создаваемый в infra-кластере
- [Мультикластерное развёртывание](../user-guide/multi-cluster.md) — пошаговое руководство
