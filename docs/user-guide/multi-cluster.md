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
      name: {{ .Values.spec.name }}
    spec:
      path: "helm-chart-sources/{{ .Values.spec.name }}"
      pluginName: helm-with-values
      repoURL: "https://github.com/org/helm-charts"
      version: "{{ .Values.spec.version }}"
      releaseName: {{ .Values.spec.name }}
      targetCluster: "{{ .Values.spec.cluster }}"
      targetNamespace: "{{ .Values.spec.name }}-system"
      backend:
        type: argocd
        namespace: argocd
      variables:
        cluster_name: "{{ .Values.spec.cluster }}"
```

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
  name: cilium
  version: v1.17.4
  cluster: client-cluster-01
  credentialRef:
    name: infra-kubeconfig
  templateRef:
    name: cilium-v1.17.4
```

```bash
kubectl apply -f addonclaim.yaml
```

## 4. Мониторинг статуса

### Просмотр AddonClaim

```bash
kubectl get addonclaim -n tenant-a
```

```
NAME     ADDON    VERSION    CLUSTER              READY   AGE
cilium   cilium   v1.17.4    client-cluster-01    True    5m
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

## 5. Обновление версии

Обновите поле `version` в AddonClaim:

```bash
kubectl patch addonclaim cilium -n tenant-a --type merge -p '{"spec":{"version":"v1.18.0"}}'
```

Контроллер перерендерит шаблон с новой версией и обновит Addon в infra-кластере.

Если используется другой шаблон для новой версии, обновите также `templateRef`:

```bash
kubectl patch addonclaim cilium -n tenant-a --type merge \
  -p '{"spec":{"version":"v1.18.0","templateRef":{"name":"cilium-v1.18.0"}}}'
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
  values:
    prometheus:
      replicas: 2
```

### Через valuesString (YAML строка)

```yaml
spec:
  valuesString: |
    prometheus:
      replicas: 2
    grafana:
      enabled: true
```

`values` и `valuesString` взаимоисключающие. Values передаются как AddonValue в infra-кластер с именем `<addon-name>-claim-values`.

## См. также

- [AddonClaim](../concepts/addon-claim.md) — описание ресурса
- [AddonTemplate](../concepts/addon-template.md) — описание шаблонов
- [Пример мультикластерного развёртывания](../examples/multi-cluster/) — готовый пример
