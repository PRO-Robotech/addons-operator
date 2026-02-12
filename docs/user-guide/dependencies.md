# Зависимости

Это руководство объясняет как настраивать зависимости между аддонами для контроля порядка развёртывания.

## Обзор

Используйте `initDependencies` для задержки развёртывания аддона до готовности зависимостей. Это обеспечивает:

- Развёртывание требуемых аддонов в первую очередь
- Существование внешних ресурсов перед развёртыванием
- Правильный порядок инициализации

## Базовая зависимость

Ожидание готовности другого аддона:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: my-app
spec:
  chart: my-app
  repoURL: https://charts.example.com
  version: "1.0.0"
  targetCluster: in-cluster
  targetNamespace: default
  backend:
    type: argocd
    namespace: argocd
  initDependencies:
    - name: cert-manager
      criteria:
        - jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
```

## Статус зависимости

Когда зависимости не выполнены:

```yaml
status:
  conditions:
    - type: Ready
      status: "False"
      reason: WaitingForDependencies
      message: "Waiting for addon: cert-manager"
    - type: DependenciesMet
      status: "False"
```

Когда зависимости удовлетворены:

```yaml
status:
  conditions:
    - type: DependenciesMet
      status: "True"
```

## Несколько зависимостей

Все зависимости должны быть готовы (логика AND):

```yaml
spec:
  initDependencies:
    - name: cert-manager
      criteria:
        - jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
    - name: external-secrets
      criteria:
        - jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
    - name: cilium
      criteria:
        - jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
```

## Зависимость с Criteria

Проверка конкретных условий зависимости:

```yaml
spec:
  initDependencies:
    - name: cert-manager
      criteria:
        - jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
```

## Зависимость от внешнего ресурса

Ожидание не-addon ресурсов (через `source` в criteria):

```yaml
spec:
  initDependencies:
    - name: external-dependency
      criteria:
        - source:
            apiVersion: v1
            kind: Secret
            name: database-credentials
            namespace: default
          jsonPath: $.data.password
          operator: Exists
```

## Паттерны зависимостей

### Инфраструктура в первую очередь

```yaml
# Сетевой слой
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: cilium
spec:
  chart: cilium
  # Нет зависимостей — развёртывается первым

---
# Слой безопасности — зависит от сети
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: cert-manager
spec:
  chart: cert-manager
  initDependencies:
    - name: cilium
      criteria:
        - jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"

---
# Приложение — зависит от безопасности
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: my-app
spec:
  chart: my-app
  initDependencies:
    - name: cert-manager
      criteria:
        - jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
```

### Ожидание CRD

```yaml
spec:
  initDependencies:
    - name: cert-manager
      criteria:
        # Проверка готовности cert-manager
        - jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
        # Ожидание существования ClusterIssuer CRD
        - source:
            apiVersion: apiextensions.k8s.io/v1
            kind: CustomResourceDefinition
            name: clusterissuers.cert-manager.io
          jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
```

### Ожидание конкретного ресурса

```yaml
spec:
  initDependencies:
    - name: cert-manager
      criteria:
        - jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
    - name: cluster-issuer-ready
      criteria:
        - source:
            apiVersion: cert-manager.io/v1
            kind: ClusterIssuer
            name: letsencrypt-prod
          jsonPath: $.status.conditions[0].type
          operator: Equal
          value: "Ready"
        - source:
            apiVersion: cert-manager.io/v1
            kind: ClusterIssuer
            name: letsencrypt-prod
          jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
```

## Отладка зависимостей

Проверка статуса зависимостей:

```bash
# Просмотр condition DependenciesMet
kubectl get addon my-app -o jsonpath='{.status.conditions[?(@.type=="DependenciesMet")]}'

# Проверка готовности аддона-зависимости
kubectl get addon cert-manager -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'
```

## Избегание циклических зависимостей

Контроллер не определяет циклические зависимости. Убедитесь, что граф зависимостей — DAG (направленный ациклический граф):

```
Правильно:
  A → B → C

Неправильно:
  A → B → C → A  (цикл!)
```

## Зависимость vs AddonPhase

| Функция | initDependencies | AddonPhase |
|---------|------------------|------------|
| Назначение | Блокировка развёртывания | Условные values |
| Эффект | Аддон не развернётся | Values активируются |
| Применение | Порядок инициализации | Активация фич |

Используйте `initDependencies` когда аддон **не может развернуться** без зависимости.

Используйте `AddonPhase` когда аддон **может развернуться**, но нуждается в разных values в зависимости от состояния.

## Следующие шаги

- [Условное развёртывание](conditional-deployment.md) — AddonPhase для условных values
- [Справочник операторов Criteria](../reference/criteria-operators.md) — доступные операторы
