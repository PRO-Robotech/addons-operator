# AddonClaim

AddonClaim представляет запрос на развёртывание аддона в удалённом infra-кластере.

## Обзор

AddonClaim:
- Определяет **имя** Addon-ресурса в удалённом кластере (`addon.name`) — явно, неизменяемо
- Определяет **какой шаблон** использовать (`templateRef`)
- Определяет **где** развернуть (kubeconfig через `credentialRef`)
- Определяет **параметры** для шаблона (`variables`)
- Определяет **конфигурацию** Helm values (`values` или `valuesString`)
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
| `addon` | [AddonIdentity](#addonidentity) | Да | Идентификация Addon-ресурса в удалённом кластере. Поле `name` обязательно и **неизменяемо** после создания |
| `templateRef` | [TemplateRef](#templateref) | Да | Ссылка на AddonTemplate для рендеринга |
| `credentialRef` | [CredentialRef](#credentialref) | Да | Ссылка на Secret с kubeconfig infra-кластера |
| `variables` | object | Нет | Произвольные параметры для рендеринга шаблона. Доступны как `.Vars.<key>` или `.Values.spec.variables.<key>` |
| `values` | object | Нет | Helm values как JSON объект |
| `valuesString` | string | Нет | Helm values как YAML строка (может содержать Go templates) |
| `valueLabels` | string | Нет | Переопределение метки `addons.in-cloud.io/values` на AddonValue. По умолчанию `"claim"` (макс. 63 символа) |

`values` и `valuesString` взаимоисключающие — можно указать только одно из двух.

### AddonIdentity

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `name` | string | Да | Имя Addon-ресурса в удалённом кластере (1–253 символа). **Неизменяемо** после создания |

Контроллер всегда использует `spec.addon.name` как имя Addon в удалённом кластере, переопределяя значение `metadata.name` из отрендеренного шаблона. Это гарантирует стабильную идентификацию Addon независимо от содержимого шаблона.

### CredentialRef

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `name` | string | Да | Имя Secret с kubeconfig (ключ: `value`) |

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
| `ready` | *bool | Удалённый Addon в состоянии Ready. `nil` = статус неизвестен, `false` = не готов, `true` = готов |
| `deployed` | bool | Удалённый Addon был развёрнут хотя бы один раз |
| `remoteAddonStatus` | [RemoteAddonStatus](#remoteaddonstatus) | Зеркало статуса Addon из infra-кластера |
| `observedGeneration` | int64 | Последнее обработанное поколение spec |
| `conditions` | []Condition | Текущее состояние AddonClaim |

### CAPI-совместимые поля

Когда на AddonClaim установлена аннотация `external-status/type: controlplane`, контроллер дополнительно заполняет поля, совместимые с [Cluster API](https://cluster-api.sigs.k8s.io/) control plane contract:

| Поле | Тип | Описание |
|------|-----|----------|
| `initialized` | *bool | Control plane инициализирован (CAPI v1beta1). Отражает condition `Deployed` из remote Addon |
| `initialization.controlPlaneInitialized` | *bool | Control plane инициализирован (CAPI v1beta2) |
| `externalManagedControlPlane` | *bool | Всегда `true` — control plane управляется внешне |
| `version` | string | Версия Kubernetes из `variables.version` |

Без аннотации эти поля остаются пустыми (`nil`/`""`).

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
  # CAPI fields (при наличии аннотации external-status/type)
  initialized: true
  externalManagedControlPlane: true
  initialization:
    controlPlaneInitialized: true
  version: "1.28.0"
  # Remote addon status
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
  addon:
    name: cilium
  credentialRef:
    name: infra-kubeconfig
  templateRef:
    name: cilium-v1.17.4
  variables:
    version: v1.17.4
    cluster: client-cluster-01
```

### С values (JSON объект)

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonClaim
metadata:
  name: monitoring
  namespace: tenant-a
spec:
  addon:
    name: monitoring
  credentialRef:
    name: infra-kubeconfig
  templateRef:
    name: monitoring-v1
  variables:
    version: "1.0.0"
    cluster: client-cluster-01
  values:
    prometheus:
      replicas: 2
    grafana:
      enabled: true
```

### С valuesString (YAML строка)

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonClaim
metadata:
  name: monitoring
  namespace: tenant-a
spec:
  addon:
    name: monitoring
  credentialRef:
    name: infra-kubeconfig
  templateRef:
    name: monitoring-v1
  variables:
    version: "1.0.0"
    cluster: client-cluster-01
  valuesString: |
    prometheus:
      replicas: 2
    grafana:
      enabled: true
```

### С CAPI интеграцией

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonClaim
metadata:
  name: k8s-control-plane
  namespace: tenant-a
  annotations:
    external-status/type: controlplane
spec:
  addon:
    name: k8s-cp
  credentialRef:
    name: infra-kubeconfig
  templateRef:
    name: control-plane-v1
  variables:
    version: "1.28.0"
    cluster: client-cluster-01
```

При наличии аннотации `external-status/type: controlplane` контроллер заполнит CAPI-совместимые поля в status, которые Cluster API может читать через unstructured JSON paths:
- `status.initialized`
- `status.externalManagedControlPlane`
- `status.initialization.controlPlaneInitialized`
- `status.version`

### С пользовательской меткой values

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonClaim
metadata:
  name: cilium
  namespace: tenant-a
spec:
  addon:
    name: cilium
  credentialRef:
    name: infra-kubeconfig
  templateRef:
    name: cilium-v1.17.4
  variables:
    version: v1.17.4
    cluster: client-cluster-01
  valueLabels: tenant  # AddonValue получит метку addons.in-cloud.io/values=tenant
  values:
    hubble:
      enabled: true
```

## Variables и шаблоны

Поле `variables` содержит произвольные параметры, доступные в AddonTemplate:

| Путь в шаблоне | Описание |
|----------------|----------|
| `.Vars.<key>` | Быстрый доступ к переменной (рекомендуется) |
| `{{ index .Values.spec.variables "<key>" }}` | Полный путь через `.Values` |
| `.Values.spec.addon.name` | Имя Addon в удалённом кластере |
| `.Values.metadata.name` | Имя AddonClaim |
| `.Values.metadata.namespace` | Namespace AddonClaim |

Пример шаблона в AddonTemplate:

```yaml
spec:
  template: |
    apiVersion: addons.in-cloud.io/v1alpha1
    kind: Addon
    metadata:
      name: placeholder  # будет переопределено значением spec.addon.name из AddonClaim
    spec:
      repoURL: "https://github.com/org/charts"
      version: "{{ .Vars.version }}"
      targetCluster: "{{ .Vars.cluster }}"
      targetNamespace: "{{ .Vars.name }}-system"
      backend:
        type: argocd
        namespace: argocd
```

> **Примечание:** Значение `metadata.name` в шаблоне всегда переопределяется значением `spec.addon.name` из AddonClaim. Шаблон может содержать произвольное значение — контроллер использует только `spec.addon.name`.

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

1. Пользователь создаёт AddonClaim с явным `spec.addon.name`
2. Контроллер рендерит AddonTemplate → Addon YAML
3. Контроллер переопределяет `metadata.name` отрендеренного Addon значением `spec.addon.name`
4. Контроллер подключается к infra-кластеру через kubeconfig
5. Контроллер создаёт/обновляет AddonValue и Addon в infra-кластере
6. Контроллер периодически опрашивает статус удалённого Addon
7. При удалении AddonClaim — удаляет Addon и AddonValue из infra-кластера

## Связанные ресурсы

- [AddonTemplate](addon-template.md) — шаблоны для генерации Addon
- [Addon](addon.md) — ресурс, создаваемый в infra-кластере
- [Мультикластерное развёртывание](../user-guide/multi-cluster.md) — пошаговое руководство
