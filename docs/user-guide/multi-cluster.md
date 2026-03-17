# Мультикластерное развёртывание

Это руководство описывает развёртывание аддонов в удалённые infra-кластеры через AddonClaim.

## Предварительные требования

- **System-кластер**: запущен addonclaim-controller
- **Infra-кластер**: запущен addons-operator (Addon controller), установлены CRD для Addon и AddonValue
- Kubeconfig для доступа из system в infra-кластер

## 1. Создание kubeconfig Secret

Создайте Secret с kubeconfig infra-кластера. Ключ должен быть `value`:

```bash
kubectl create secret generic infra-kubeconfig \
  --namespace=tenant-a \
  --from-file=value=/path/to/infra-cluster-kubeconfig
```

Или через YAML:

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

Secret должен находиться в том же namespace, что и AddonClaim.

## 2. Создание AddonTemplate

AddonTemplate — cluster-scoped ресурс с Go template для генерации Addon:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonTemplate
metadata:
  name: cilium-v1.17.4
  labels:
    name.addons.in-cloud.io: cilium
    version.addons.in-cloud.io: v1.17.4
spec:
  template: |
    apiVersion: addons.in-cloud.io/v1alpha1
    kind: Addon
    metadata:
      name: placeholder  # переопределяется значением spec.addon.name из AddonClaim
    spec:
      path: "helm-chart-sources/{{ .Vars.name }}"
      pluginName: helm-with-values
      repoURL: "https://github.com/org/helm-charts"
      version: "{{ .Vars.version }}"
      releaseName: {{ .Vars.name }}
      targetCluster: "{{ .Vars.cluster }}"
      targetNamespace: "{{ .Vars.name }}-system"
      backend:
        type: argocd
        namespace: argocd
      variables:
        cluster_name: "{{ .Vars.cluster }}"
```

Шаблон получает AddonClaim как контекст. Переменные из `spec.variables` доступны через `.Vars.<key>`.

> **Примечание:** Поле `metadata.name` в шаблоне всегда переопределяется контроллером значением `spec.addon.name` из AddonClaim. Шаблон определяет спецификацию Addon, а идентификация задаётся явно через `spec.addon.name`.

```bash
kubectl apply -f addontemplate.yaml
```

## 3. Создание AddonClaim

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonClaim
metadata:
  name: cilium
  namespace: tenant-a
spec:
  addon:
    name: cilium              # имя Addon в infra-кластере (неизменяемо)
  credentialRef:
    name: infra-kubeconfig
  templateRef:
    name: cilium-v1.17.4
  variables:
    version: v1.17.4
    cluster: client-cluster-01
```

> **Важно:** Поле `spec.addon.name` задаёт имя Addon-ресурса в удалённом кластере и является **неизменяемым** после создания. Контроллер переопределяет `metadata.name` из отрендеренного шаблона значением `spec.addon.name`.

```bash
kubectl apply -f addonclaim.yaml
```

## 4. Мониторинг статуса

### Просмотр AddonClaim

```bash
kubectl get addonclaim -n tenant-a
```

```
NAME     ADDON    READY   AGE
cilium   cilium   True    5m
```

### Просмотр conditions

```bash
kubectl get addonclaim cilium -n tenant-a -o jsonpath='{range .status.conditions[*]}{.type}: {.status} ({.reason}){"\n"}{end}'
```

```
Ready: True (FullyReconciled)
Progressing: False (Complete)
Degraded: False (Healthy)
TemplateRendered: True (Rendered)
RemoteConnected: True (Connected)
AddonSynced: True (Synced)
```

### Проверка удалённого статуса

```bash
kubectl get addonclaim cilium -n tenant-a -o jsonpath='{.status.remoteAddonStatus}'
```

### Проверка ресурсов в infra-кластере

```bash
kubectl --kubeconfig=/path/to/infra-kubeconfig get addon cilium
kubectl --kubeconfig=/path/to/infra-kubeconfig get addonvalue cilium-claim-values
```

## 5. Обновление

Обновите переменные в AddonClaim:

```bash
kubectl patch addonclaim cilium -n tenant-a --type merge \
  -p '{"spec":{"variables":{"version":"v1.18.0"}}}'
```

Контроллер перерендерит шаблон с новыми переменными и обновит Addon в infra-кластере.

Если используется другой шаблон для новой версии, обновите также `templateRef`:

```bash
kubectl patch addonclaim cilium -n tenant-a --type merge \
  -p '{"spec":{"variables":{"version":"v1.18.0"},"templateRef":{"name":"cilium-v1.18.0"}}}'
```

## 6. Удаление и cleanup

При удалении AddonClaim контроллер автоматически удаляет Addon и AddonValue из infra-кластера через finalizer:

```bash
kubectl delete addonclaim cilium -n tenant-a
```

Порядок очистки:
1. Контроллер удаляет Addon в infra-кластере
2. Контроллер удаляет AddonValue в infra-кластере
3. Финализатор снимается
4. AddonClaim удаляется из system-кластера

Если соединение с infra-кластером недоступно при удалении, контроллер логирует предупреждение и всё равно снимает финализатор, чтобы не блокировать удаление.

## 7. Настройка polling interval

Контроллер периодически опрашивает статус удалённого Addon. По умолчанию интервал — 15 секунд.

Для изменения передайте флаг `--polling-interval` при запуске контроллера:

```bash
# Опрос каждые 30 секунд
addonclaim-controller --polling-interval=30s

# Опрос каждую минуту
addonclaim-controller --polling-interval=1m
```

В Kubernetes Deployment:

```yaml
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --polling-interval=30s
```

## Добавление values

### Через values (JSON объект)

```yaml
spec:
  addon:
    name: cilium
  variables:
    version: v1.17.4
    cluster: client-cluster-01
  values:
    hubble:
      enabled: true
      relay:
        enabled: true
```

### Через valuesString (YAML строка)

```yaml
spec:
  addon:
    name: monitoring
  variables:
    version: "1.0.0"
    cluster: client-cluster-01
  valuesString: |
    prometheus:
      replicas: 2
    grafana:
      enabled: true
```

`values` и `valuesString` взаимоисключающие. Values передаются как AddonValue в infra-кластер с именем `<addon-name>-claim-values`.

### Пользовательская метка values

По умолчанию AddonValue создаётся с меткой `addons.in-cloud.io/values=claim`. Для переопределения используйте `valueLabels`:

```yaml
spec:
  valueLabels: tenant
  values:
    custom: config
```

AddonValue будет создан с именем `<addon-name>-tenant-values` и меткой `addons.in-cloud.io/values=tenant`.

## CAPI интеграция

AddonClaim может выступать внешним control plane провайдером для [Cluster API](https://cluster-api.sigs.k8s.io/). Для активации добавьте аннотацию:

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

Контроллер заполнит CAPI-совместимые поля в status:

| Поле | Описание |
|------|----------|
| `status.initialized` | `true` если remote Addon имеет `status.deployed=true` (latching bool — не сбрасывается) |
| `status.initialization.controlPlaneInitialized` | Дублирует `initialized` для CAPI v1beta2 |
| `status.externalManagedControlPlane` | Всегда `true` |
| `status.version` | Значение из `variables.version` |

Без аннотации эти поля остаются пустыми.

## См. также

- [AddonClaim](../concepts/addon-claim.md) — описание ресурса
- [AddonTemplate](../concepts/addon-template.md) — описание шаблонов
- [Пример мультикластерного развёртывания](../examples/multi-cluster/) — готовый пример
