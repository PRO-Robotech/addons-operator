# Мониторинг

Это руководство описывает мониторинг здоровья аддонов и отладку проблем.

## Status Conditions

Аддоны сообщают состояние через Kubernetes conditions:

| Condition | Описание |
|-----------|----------|
| `Ready` | Общее здоровье аддона |
| `Progressing` | Выполняется reconciliation |
| `Degraded` | Невосстановимая ошибка |
| `DependenciesMet` | Зависимости удовлетворены |
| `ValuesResolved` | Агрегация values завершена |
| `ApplicationCreated` | Argo CD Application существует |
| `Synced` | Application синхронизирован |
| `Healthy` | Application здоров |

## Быстрая проверка статуса

```bash
# Все аддоны со статусом Ready
kubectl get addons -A -o wide

# Conditions конкретного аддона
kubectl get addon prometheus -o jsonpath='{range .status.conditions[*]}{.type}: {.status} ({.reason}){"\n"}{end}'
```

## Детали Condition

```bash
# Полный condition с сообщением
kubectl get addon prometheus -o jsonpath='{.status.conditions[?(@.type=="Ready")]}'
```

Вывод:

```json
{
  "type": "Ready",
  "status": "True",
  "reason": "FullyReconciled",
  "message": "Addon is ready",
  "lastTransitionTime": "2024-01-15T10:00:00Z",
  "observedGeneration": 2
}
```

## Здоровый аддон

Все conditions должны быть:

```
Ready: True
DependenciesMet: True
ValuesResolved: True
ApplicationCreated: True
Synced: True
Healthy: True
```

## Нездоровые состояния

### Ожидание зависимостей

```yaml
conditions:
  - type: Ready
    status: "False"
    reason: WaitingForDependencies
  - type: DependenciesMet
    status: "False"
    message: "Waiting for addon: cert-manager"
```

Решение: Проверьте статус аддона-зависимости.

### Ошибка разрешения Values

```yaml
conditions:
  - type: Ready
    status: "False"
    reason: ValuesResolutionFailed
  - type: ValuesResolved
    status: "False"
    message: "Secret not found: my-secret"
```

Решение: Создайте отсутствующий Secret/ConfigMap или исправьте AddonValue.

### Ошибка синхронизации

```yaml
conditions:
  - type: Ready
    status: "False"
    reason: SyncFailed
  - type: Synced
    status: "False"
    message: "ComparisonError"
```

Решение: Проверьте Argo CD Application на наличие ошибок синхронизации.

### Application нездоров

```yaml
conditions:
  - type: Ready
    status: "False"
    reason: Unhealthy
  - type: Healthy
    status: "False"
    message: "Deployment prometheus has 0/1 replicas"
```

Решение: Проверьте развёрнутые ресурсы на наличие проблем.

## Команды для отладки

### Проверка Addon

```bash
# Полный статус аддона
kubectl get addon prometheus -o yaml

# События
kubectl describe addon prometheus

# Итоговые values (в Argo CD Application)
kubectl get application -n argocd prometheus -o jsonpath='{.spec.source.helm.values}' | yq
```

### Проверка AddonPhase

```bash
# Статус вычисления rules
kubectl get addonphase my-app -o jsonpath='{.status.ruleStatuses}'

# Какие rules сработали
kubectl get addonphase my-app -o jsonpath='{.status.ruleStatuses[?(@.matched==true)].name}'
```

### Проверка Argo CD Application

```bash
# Статус Application
kubectl get application -n argocd prometheus -o yaml

# Статус синхронизации
kubectl get application -n argocd prometheus -o jsonpath='{.status.sync}'

# Статус здоровья
kubectl get application -n argocd prometheus -o jsonpath='{.status.health}'

# Здоровье ресурсов
kubectl get application -n argocd prometheus -o jsonpath='{.status.resources[*].health.status}'
```

### Проверка логов контроллера

```bash
kubectl logs -n addon-operator-system deployment/addon-operator-controller-manager -f
```

## Метрики

Контроллер экспортирует Prometheus метрики на `:8080/metrics`:

| Метрика | Описание |
|---------|----------|
| `controller_runtime_reconcile_total` | Всего reconciliations |
| `controller_runtime_reconcile_errors_total` | Ошибки reconciliation |
| `controller_runtime_reconcile_time_seconds` | Длительность reconciliation |
| `workqueue_depth` | Элементы в очереди |

### Конфигурация сбора

```yaml
apiVersion: v1
kind: Service
metadata:
  name: addon-operator-metrics
  namespace: addon-operator-system
  labels:
    prometheus.io/scrape: "true"
spec:
  ports:
    - name: metrics
      port: 8080
```

## Алертинг

Пример Prometheus alerts:

```yaml
groups:
  - name: addon-operator
    rules:
      - alert: AddonNotReady
        expr: |
          kube_customresource_addon_status_condition{condition="Ready",status="False"} == 1
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Addon {{ $labels.name }} not ready"

      - alert: AddonReconcileErrors
        expr: |
          rate(controller_runtime_reconcile_errors_total{controller="addon"}[5m]) > 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Addon reconciliation errors"
```

## Чеклист для устранения неполадок

1. **Проверьте conditions аддона** — `kubectl get addon <имя> -o yaml`
2. **Проверьте события** — `kubectl describe addon <имя>`
3. **Проверьте зависимости** — Все `initDependencies` аддоны готовы?
4. **Проверьте values** — Существуют ли указанные Secrets/ConfigMaps?
5. **Проверьте Argo CD Application** — Синхронизирован и здоров?
6. **Проверьте логи контроллера** — Есть ли ошибки при reconciliation?

## Следующие шаги

- [Устранение неполадок](../troubleshooting.md) — частые проблемы и решения
- [Справочник операторов Criteria](../reference/criteria-operators.md) — отладка criteria
