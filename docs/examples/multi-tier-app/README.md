# Пример многоуровневого приложения

Этот пример демонстрирует развёртывание стека многоуровневого приложения с упорядоченными зависимостями с использованием [podinfo](https://github.com/stefanprodan/podinfo).

## Архитектура

```
┌─────────────┐
│  Frontend   │ ─── зависит от ──┐
└─────────────┘                  │
                                 ▼
                        ┌─────────────┐
                        │   Backend   │
                        └─────────────┘
                               │
                               │ зависит от
                               ▼
                        ┌─────────────┐
                        │    Cache    │
                        └─────────────┘
```

## Ресурсы

### 1. Кэш (базовый сервис)

```yaml
# cache-addon.yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: cache
spec:
  chart: podinfo
  repoURL: https://stefanprodan.github.io/podinfo
  version: "6.5.0"
  targetCluster: in-cluster
  targetNamespace: multi-tier
  backend:
    namespace: argocd
    syncPolicy:
      automated:
        prune: true
        selfHeal: true
      syncOptions:
        - CreateNamespace=true
  valuesSelectors:
    - name: config
      priority: 0
      matchLabels:
        addons.in-cloud.io/addon: cache
---
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: cache-config
  labels:
    addons.in-cloud.io/addon: cache
spec:
  values:
    replicaCount: 1
    ui:
      message: "Cache Service"
```

### 2. Backend (зависит от Cache)

```yaml
# backend-addon.yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: backend
spec:
  chart: podinfo
  repoURL: https://stefanprodan.github.io/podinfo
  version: "6.5.0"
  targetCluster: in-cluster
  targetNamespace: multi-tier
  backend:
    namespace: argocd
    syncPolicy:
      automated:
        prune: true
        selfHeal: true
  valuesSelectors:
    - name: config
      priority: 0
      matchLabels:
        addons.in-cloud.io/addon: backend
  variables:
    environment: production
  initDependencies:
    - name: cache
      criteria:
        - jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
---
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: backend-config
  labels:
    addons.in-cloud.io/addon: backend
spec:
  values:
    replicaCount: 2
    ui:
      message: "Backend API ({{ .Variables.environment }})"
```

### 3. Frontend (зависит от Backend)

```yaml
# frontend-addon.yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: frontend
spec:
  chart: podinfo
  repoURL: https://stefanprodan.github.io/podinfo
  version: "6.5.0"
  targetCluster: in-cluster
  targetNamespace: multi-tier
  backend:
    namespace: argocd
    syncPolicy:
      automated:
        prune: true
        selfHeal: true
  valuesSelectors:
    - name: config
      priority: 0
      matchLabels:
        addons.in-cloud.io/addon: frontend
  initDependencies:
    - name: backend
      criteria:
        - jsonPath: $.status.conditions[0].status
          operator: Equal
          value: "True"
---
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonValue
metadata:
  name: frontend-config
  labels:
    addons.in-cloud.io/addon: frontend
spec:
  values:
    replicaCount: 1
    ui:
      message: "Frontend App"

## Порядок развёртывания

Оператор автоматически обрабатывает порядок развёртывания на основе зависимостей:

1. **Cache** развёртывается первым (нет зависимостей)
2. **Backend** ожидает готовности Cache
3. **Frontend** ожидает готовности Backend

```bash
# Применить все ресурсы — порядок не важен!
kubectl apply -f cache-addon.yaml
kubectl apply -f backend-addon.yaml
kubectl apply -f frontend-addon.yaml
```

## Мониторинг развёртывания

```bash
# Наблюдать за всеми аддонами
kubectl get addons -w

# Проверить статус зависимостей backend
kubectl get addon backend -o jsonpath='{.status.conditions[?(@.type=="DependenciesMet")]}'

# Просмотреть детальный статус
kubectl describe addon backend
```

## Ожидаемый процесс

```
Время 0с:  cache: Progressing
           backend: WaitingForDependency, frontend: WaitingForDependency

Время 30с: cache: Ready
           backend: Progressing, frontend: WaitingForDependency

Время 60с: cache: Ready, backend: Ready
           frontend: Progressing

Время 90с: Все Ready
```

## Проверка

```bash
# Проверить все поды
kubectl get pods -n multi-tier

# Проверить что template переменные отрендерены
kubectl get application -n argocd backend -o jsonpath='{.spec.source.helm.values}' | grep message
# Ожидается: "Backend API (production)"
```

## Очистка

```bash
kubectl delete addon frontend backend cache
kubectl delete addonvalue frontend-config backend-config cache-config
```
