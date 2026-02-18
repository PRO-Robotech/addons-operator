# Развёртывание аддонов

Это руководство объясняет как создавать и управлять Addon ресурсами.

## Базовый Addon

Addon определяет какой Helm chart развернуть:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: nginx-ingress  # Addon cluster-scoped, без namespace
spec:
  chart: ingress-nginx
  repoURL: https://kubernetes.github.io/ingress-nginx
  version: "4.8.3"
  targetCluster: in-cluster
  targetNamespace: ingress-nginx
  backend:
    type: argocd
    namespace: argocd
```

Применение:

```bash
kubectl apply -f nginx-addon.yaml
```

## Addon с выбором Values

Выбор конфигурации из AddonValue ресурсов:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: prometheus
spec:
  chart: kube-prometheus-stack
  repoURL: https://prometheus-community.github.io/helm-charts
  version: "55.5.0"
  targetCluster: in-cluster
  targetNamespace: monitoring
  backend:
    type: argocd
    namespace: argocd
  valuesSelectors:
    - name: base
      priority: 0
      matchLabels:
        addons.in-cloud.io/addon: prometheus
        addons.in-cloud.io/layer: base
    - name: environment
      priority: 10
      matchLabels:
        addons.in-cloud.io/addon: prometheus
        addons.in-cloud.io/environment: production
```

## Спецификация Addon

| Поле | Обязательно | Описание |
|------|-------------|----------|
| `chart` | Да* | Имя Helm chart |
| `path` | Нет* | Путь к директории с чартом (для Git) |
| `repoURL` | Да | URL Helm или Git репозитория |
| `version` | Да | Версия chart или Git ревизия |
| `targetCluster` | Да | Целевой кластер (`in-cluster` или имя кластера) |
| `targetNamespace` | Да | Namespace для развёртывания |
| `backend.type` | Нет | Тип бэкенда (по умолчанию: `argocd`) |
| `backend.namespace` | Да | Namespace бэкенда |
| `pluginName` | Нет | ArgoCD Config Management Plugin (вместо Helm) |
| `releaseName` | Нет | Переопределение имени Helm release |
| `valuesSelectors` | Нет | Список селекторов values |
| `valuesSources` | Нет | Прямые источники values (Secret/ConfigMap) |
| `variables` | Нет | Переменные для шаблонов |
| `initDependencies` | Нет | Зависимости |
| `finalizer` | Нет | Каскадное удаление ресурсов (`true`/`false`) |

\* Должен быть указан либо `chart`, либо `path`, но не оба одновременно.

## Проверка статуса Addon

Просмотр статуса аддона:

```bash
kubectl get addon prometheus -o yaml
```

Ключевые поля статуса:

```yaml
status:
  conditions:
    - type: Ready
      status: "True"
      reason: FullyReconciled
    - type: DependenciesMet
      status: "True"
    - type: ValuesResolved
      status: "True"
    - type: ApplicationCreated
      status: "True"
    - type: Synced
      status: "True"
    - type: Healthy
      status: "True"
  applicationRef:
    name: prometheus
    namespace: argocd
  valuesHash: "sha256:abc123..."  # Хеш для отслеживания изменений
```

> **Примечание:** Итоговые values хранятся в Argo CD Application:
> ```bash
> kubectl get application -n argocd prometheus -o jsonpath='{.spec.source.helm.values}'
> ```

## Быстрая проверка статуса

```bash
# Расширенный вывод показывает статус Ready
kubectl get addon prometheus -o wide

# Проверка конкретного condition
kubectl get addon prometheus -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'
```

## Config Management Plugin

Для использования ArgoCD Config Management Plugin вместо встроенного Helm укажите `pluginName`:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: secrets-app
spec:
  chart: my-chart
  repoURL: https://example.com/charts
  version: "1.0.0"
  targetCluster: in-cluster
  targetNamespace: my-app
  pluginName: helm-secrets
  backend:
    type: argocd
    namespace: argocd
```

В режиме Plugin values передаются через переменную окружения `HELM_VALUES` (base64-encoded YAML), а не через `source.helm.values`.

## Переопределение Release Name

По умолчанию имя Helm release совпадает с именем Addon. Чтобы задать другое имя:

```yaml
spec:
  releaseName: custom-release
```

Работает как в Helm режиме (`source.helm.releaseName`), так и в Plugin режиме (переменная окружения `RELEASE_NAME`).

## Обновление Addon

Обновление версии:

```bash
kubectl patch addon prometheus --type=merge -p '{"spec":{"version":"55.6.0"}}'
```

Или редактирование напрямую:

```bash
kubectl edit addon prometheus
```

## Удаление Addon

```bash
kubectl delete addon prometheus
```

Это удалит связанный Argo CD Application.

### Каскадное удаление ресурсов

По умолчанию при удалении Addon удаляется только объект Application, а развёрнутые ресурсы (Deployments, Services и т.д.) **остаются** в кластере. Чтобы Argo CD удалил все созданные ресурсы вместе с Application, укажите `finalizer: true`:

```yaml
spec:
  finalizer: true
```

| `finalizer` | Поведение при удалении |
|-------------|----------------------|
| `true` | Argo CD удалит все ресурсы, затем удалит Application |
| `false` / не задано | Удаляется только объект Application, ресурсы остаются |

> **Важно:** Каскадное удаление может занять время, так как Argo CD ожидает удаления всех управляемых ресурсов. Addon будет оставаться в состоянии удаления до завершения процесса.

## Мульти-кластерное развёртывание

Развёртывание в удалённый кластер:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: prometheus-remote
spec:
  chart: kube-prometheus-stack
  repoURL: https://prometheus-community.github.io/helm-charts
  version: "55.5.0"
  targetCluster: production-cluster  # Имя кластера в Argo CD
  targetNamespace: monitoring
  backend:
    type: argocd
    namespace: argocd
```

`targetCluster` должен соответствовать кластеру, зарегистрированному в Argo CD.

## Следующие шаги

- [Управление values](managing-values.md) — настройка values аддона
- [Зависимости](dependencies.md) — настройка зависимостей
- [Условное развёртывание](conditional-deployment.md) — использование AddonPhase
