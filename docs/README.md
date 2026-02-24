# Документация Addon Operator

Addon Operator автоматизирует управление жизненным циклом Kubernetes-аддонов через декларативные CRD и интеграцию с Argo CD.

## Обзор

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Addon     │────▶│   Addon     │────▶│  Argo CD    │
│    CRD      │     │ Controller  │     │ Application │
└─────────────┘     └─────────────┘     └─────────────┘
       ▲                   │
       │                   ▼
┌─────────────┐     ┌─────────────┐
│ AddonValue  │     │  Агрегация  │
│    CRD      │     │   Values    │
└─────────────┘     └─────────────┘
       ▲                   ▲
       │                   │
┌─────────────┐     ┌─────────────┐
│ AddonPhase  │────▶│  Движок     │
│    CRD      │     │  Criteria   │
└─────────────┘     └─────────────┘
```

## Быстрые ссылки

| Раздел | Описание |
|--------|----------|
| [Быстрый старт](getting-started.md) | Установка оператора и развёртывание первого аддона |
| [Концепции](concepts/) | Основные ресурсы |
| [Руководство пользователя](user-guide/) | Пошаговые инструкции |
| [Справочник](reference/) | Спецификации API и операторы |
| [Примеры](examples/) | Примеры реальных развёртываний |
| [Устранение неполадок](troubleshooting.md) | Частые проблемы и решения |

## Концепции

Оператор использует три Custom Resource Definition (CRD):

### [Addon](concepts/addon.md)

Основной ресурс, определяющий какой Helm chart развернуть:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: cilium
spec:
  chart: cilium
  repoURL: https://helm.cilium.io/
  version: "1.14.5"
  targetCluster: in-cluster
  targetNamespace: kube-system
```

### [AddonValue](concepts/addon-value.md)

Хранит конфигурационные значения, которые могут выбираться Addon'ами:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: cilium-production
  labels:
    addons.in-cloud.io/addon: cilium
    addons.in-cloud.io/environment: production
spec:
  values:
    cluster:
      name: production-cluster
```

### [AddonPhase](concepts/addon-phase.md)

Включает условную инъекцию значений на основе состояния кластера:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonPhase
metadata:
  name: cilium
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
        matchLabels:
          addons.in-cloud.io/feature.tls: "true"
```

### [AddonClaim](concepts/addon-claim.md)

Запрос на развёртывание аддона в удалённом infra-кластере:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonClaim
metadata:
  name: cilium
  namespace: tenant-a
spec:
  addon:
    name: cilium                # явная, неизменяемая идентификация Addon
  credentialRef:
    name: infra-kubeconfig
  templateRef:
    name: cilium-v1.17.4
  variables:
    version: v1.17.4
    cluster: client-cluster-01
```

### [AddonTemplate](concepts/addon-template.md)

Переиспользуемый шаблон для генерации Addon из AddonClaim:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonTemplate
metadata:
  name: cilium-v1.17.4
spec:
  template: |
    apiVersion: addons.in-cloud.io/v1alpha1
    kind: Addon
    metadata:
      name: placeholder  # переопределяется spec.addon.name из AddonClaim
    spec:
      chart: cilium
      repoURL: https://helm.cilium.io
      version: "{{ .Vars.version }}"
      targetCluster: "{{ .Vars.cluster }}"
      targetNamespace: kube-system
      backend:
        type: argocd
        namespace: argocd
```

## Возможности

- **Декларативное управление** — определяйте аддоны как Kubernetes-ресурсы
- **Агрегация значений** — объединение values из нескольких AddonValue по приоритету
- **Условная конфигурация** — инъекция values на основе состояния кластера через AddonPhase
- **Управление зависимостями** — контроль порядка развёртывания через `initDependencies`
- **Интеграция с Argo CD** — GitOps для развёртывания и синхронизации аддонов
- **Поддержка шаблонов** — Go templates для динамической генерации values
- **Динамические источники данных** — извлечение values из любых Kubernetes ресурсов (Secret, ConfigMap, Deployment, CRD и др.) с автоматическим отслеживанием изменений
- **Мультикластерное управление** — развёртывание аддонов в удалённые кластеры через AddonClaim и AddonTemplate

## Архитектура

Оператор состоит из трёх контроллеров:

1. **Addon Controller**
   - Агрегирует values из AddonValue ресурсов
   - Генерирует Argo CD Application
   - Отслеживает статус развёртывания через conditions

2. **AddonPhase Controller**
   - Вычисляет rules по состоянию кластера
   - Инъектирует matching selectors в статус Addon
   - Обеспечивает phase-based конфигурацию

3. **AddonClaim Controller** (отдельный бинарник)
   - Рендерит AddonTemplate в Addon манифест
   - Создаёт Addon и AddonValue в удалённом infra-кластере
   - Зеркалирует статус удалённого Addon

## Status Conditions

Аддоны сообщают своё состояние через стандартные Kubernetes conditions:

| Condition | Описание |
|-----------|----------|
| `Ready` | Аддон полностью развёрнут и работает |
| `Progressing` | Выполняется reconciliation |
| `Degraded` | Произошла невосстановимая ошибка |
| `DependenciesMet` | Все зависимости удовлетворены |
| `ValuesResolved` | Агрегация values завершена |
| `ApplicationCreated` | Argo CD Application создан |
| `Synced` | Application синхронизирован |
| `Healthy` | Application здоров |

## Помощь

- [Устранение неполадок](troubleshooting.md) — частые проблемы и решения
- [GitHub Issues](https://github.com/PRO-Robotech/addons-operator/issues) — сообщить об ошибке или запросить функцию

## Лицензия

Apache License 2.0
