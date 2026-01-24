# Быстрый старт

Это руководство проведёт вас через развёртывание первого аддона с использованием Addon Operator.

## Требования

- Kubernetes 1.25+
- Argo CD 2.8+ установленный в namespace `argocd`
- `kubectl` настроенный для доступа к кластеру

## Установка

### 1. Клонирование репозитория

```bash
git clone https://github.com/PRO-Robotech/addons-operator.git
cd addons-operator
```

### 2. Установка CRD

```bash
make install
```

### 3. Развёртывание контроллера

```bash
make deploy IMG=<your-registry>/addons-operator:latest
```

Или для локальной разработки:

```bash
make run
```

### 4. Проверка установки

```bash
kubectl get pods -n addon-operator-system
kubectl get crd | grep addons.in-cloud.io
```

## Первый аддон

В этом примере мы развернём [podinfo](https://github.com/stefanprodan/podinfo) — легковесное демонстрационное приложение.

### Шаг 1: Создание AddonValue

AddonValue хранит конфигурационные values для аддона. Создайте файл `podinfo-values.yaml`:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: podinfo-defaults
  labels:
    addons.in-cloud.io/addon: podinfo
    addons.in-cloud.io/layer: defaults
spec:
  values:
    replicaCount: 2
    resources:
      requests:
        memory: "64Mi"
        cpu: "100m"
    ui:
      message: "Hello from {{ .Variables.environment }}!"
```

Примените его:

```bash
kubectl apply -f podinfo-values.yaml
```

### Шаг 2: Создание Addon

Addon определяет, какой Helm chart развернуть и как выбрать values. Создайте `podinfo-addon.yaml`:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: podinfo
spec:
  chart: podinfo
  repoURL: https://stefanprodan.github.io/podinfo
  version: "6.5.0"
  targetCluster: in-cluster
  targetNamespace: default
  backend:
    namespace: argocd
    syncPolicy:
      automated:
        prune: true
        selfHeal: true
  valuesSelectors:
    - name: defaults
      priority: 0
      matchLabels:
        addons.in-cloud.io/addon: podinfo
        addons.in-cloud.io/layer: defaults
  variables:
    environment: production
```

Примените его:

```bash
kubectl apply -f podinfo-addon.yaml
```

### Шаг 3: Проверка статуса

```bash
# Проверить статус Addon
kubectl get addon podinfo

# Детальный статус
kubectl get addon podinfo -o jsonpath='{range .status.conditions[*]}{.type}: {.status} ({.reason}){"\n"}{end}'
```

Ожидаемый результат после успешного развёртывания:

```
Ready: True (FullyReconciled)
Progressing: False (Complete)
Degraded: False (Healthy)
DependenciesMet: True (AllDependenciesMet)
ValuesResolved: True (ValuesResolved)
ApplicationCreated: True (ApplicationCreated)
Synced: True (Synced)
Healthy: True (Healthy)
```

### Шаг 4: Проверка развёрнутых ресурсов

```bash
# Проверить Pod
kubectl get pods -l app.kubernetes.io/name=podinfo

# Проверить Service
kubectl get svc -l app.kubernetes.io/name=podinfo

# Проверить Argo CD Application
kubectl get application podinfo -n argocd
```

### Шаг 5: Проверка рендеринга шаблона

Шаблон `{{ .Variables.environment }}` в AddonValue должен быть заменён на `production`:

```bash
kubectl get application podinfo -n argocd -o jsonpath='{.spec.source.helm.values}' | grep message
```

Ожидаемый результат:
```
  message: Hello from production!
```

## Понимание процесса

```
┌─────────────┐    выбирает   ┌─────────────┐
│   Addon     │──────────────▶│ AddonValue  │
│             │               │  (values)   │
└─────┬───────┘               └─────────────┘
      │
      │ создаёт
      ▼
┌─────────────┐
│ Application │
│  (ArgoCD)   │
└─────────────┘
```

1. **Addon** определяет что развернуть (chart, репозиторий, версия)
2. **AddonValue** хранит конфигурационные values
3. **Addon** выбирает AddonValue по селекторам меток
4. Контроллер рендерит шаблоны и создаёт **Argo CD Application** с объединёнными values

## Очистка

```bash
kubectl delete addon podinfo
kubectl delete addonvalue podinfo-defaults
```

## Следующие шаги

- [Концепции](concepts/addon.md) — понимание Addon, AddonValue и AddonPhase
- [Руководство пользователя](user-guide/) — пошаговые руководства
- [Примеры](examples/) — примеры реальных развёртываний
- [Устранение неполадок](troubleshooting.md) — типичные проблемы и решения
