# Установка

Это руководство описывает установку Addon Operator в Kubernetes кластер.

## Требования

- Kubernetes 1.28+
- kubectl с доступом к кластеру
- Argo CD 2.8+ установленный в кластере
- Go 1.21+ (для сборки из исходников)

## Установка из исходников

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

Для production:

```bash
make deploy IMG=<your-registry>/addons-operator:latest
```

Для локальной разработки (запуск вне кластера):

```bash
make run
```

## Проверка установки

Проверка запуска контроллера:

```bash
kubectl get pods -n addon-operator-system
```

Ожидаемый вывод:

```
NAME                                         READY   STATUS    RESTARTS   AGE
addon-operator-controller-manager-xxxx       1/1     Running   0          30s
```

Проверка установки CRD:

```bash
kubectl get crd | grep addons.in-cloud.io
```

Ожидаемый вывод:

```
addons.addons.in-cloud.io       2026-01-18T10:00:00Z
addonphases.addons.in-cloud.io  2026-01-18T10:00:00Z
addonvalues.addons.in-cloud.io  2026-01-18T10:00:00Z
```

## Конфигурация

### Флаги контроллера

| Флаг | По умолчанию | Описание |
|------|--------------|----------|
| `--metrics-bind-address` | `:8080` | Адрес эндпоинта метрик |
| `--health-probe-bind-address` | `:8081` | Адрес health probe |
| `--leader-elect` | `false` | Включить leader election |

> **Примечание:** Namespace для Argo CD Applications указывается в поле `spec.backend.namespace` каждого Addon ресурса. Это позволяет разным аддонам использовать разные инстансы Argo CD.

### Лимиты ресурсов

Конфигурация ресурсов по умолчанию:

```yaml
resources:
  limits:
    cpu: 500m
    memory: 128Mi
  requests:
    cpu: 10m
    memory: 64Mi
```

## RBAC

Контроллеру требуются разрешения на:

- Управление ресурсами Addon, AddonValue, AddonPhase
- Создание/обновление Argo CD Applications
- Чтение Secrets и ConfigMaps для sources значений

---

## AddonClaim Controller (мультикластерный режим)

`addonclaim-controller` — **отдельный контроллер** для управления аддонами в удалённых infra-кластерах через ресурсы AddonClaim и AddonTemplate. Запускается в system-кластере.

> **Архитектура:** addons-operator работает в infra-кластере и управляет Addon/AddonValue/AddonPhase. addonclaim-controller работает в system-кластере и управляет AddonClaim/AddonTemplate, создавая ресурсы в infra-кластерах через kubeconfig.

### Установка CRD

AddonClaim и AddonTemplate CRD устанавливаются общей командой `make install` вместе с остальными CRD.

Проверка:

```bash
kubectl get crd | grep addons.in-cloud.io
```

Ожидаемый вывод должен включать:

```
addonclaims.addons.in-cloud.io      2026-01-18T10:00:00Z
addontemplates.addons.in-cloud.io   2026-01-18T10:00:00Z
```

### Сборка образа

Для addonclaim-controller используется отдельный `Dockerfile.addonclaim`:

```bash
docker build -t <your-registry>/addonclaim-controller:latest -f Dockerfile.addonclaim .
```

### Запуск для локальной разработки

```bash
go run ./cmd/addonclaim-controller
```

### Развёртывание в кластере

addonclaim-controller деплоится отдельным Deployment в system-кластере. Минимальный манифест:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: addonclaim-controller-manager
  namespace: <namespace>
spec:
  replicas: 1
  selector:
    matchLabels:
      control-plane: addonclaim-controller-manager
  template:
    metadata:
      labels:
        control-plane: addonclaim-controller-manager
    spec:
      serviceAccountName: addonclaim-controller-manager
      containers:
      - name: manager
        image: <your-registry>/addonclaim-controller:latest
        args:
        - --leader-elect
        - --polling-interval=15s
        - --webhook-cert-path=/tmp/k8s-webhook-server/serving-certs
        ports:
        - containerPort: 9443
          name: webhook-server
          protocol: TCP
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
        volumeMounts:
        - mountPath: /tmp/k8s-webhook-server/serving-certs
          name: webhook-certs
          readOnly: true
      volumes:
      - name: webhook-certs
        secret:
          secretName: addonclaim-webhook-server-cert
```

Также необходимы:
- **Service** для webhook-сервера (порт 443 → targetPort 9443)
- **ValidatingWebhookConfiguration** для AddonClaim и AddonTemplate
- **Certificate** (cert-manager) для TLS вебхуков
- **ServiceAccount** и **RBAC** (ClusterRole/ClusterRoleBinding)

### Флаги контроллера

| Флаг | По умолчанию | Описание |
|------|--------------|----------|
| `--metrics-bind-address` | `0` (выключен) | Адрес эндпоинта метрик |
| `--health-probe-bind-address` | `:8081` | Адрес health probe |
| `--leader-elect` | `false` | Включить leader election |
| `--polling-interval` | `15s` | Интервал опроса статуса удалённого Addon |
| `--webhook-cert-path` | `` | Путь к TLS-сертификатам вебхуков |
| `--metrics-secure` | `true` | HTTPS для метрик |
| `--graceful-shutdown-timeout` | `30s` | Таймаут graceful shutdown |

### RBAC

addonclaim-controller требуются разрешения на:

- Управление ресурсами AddonClaim (get, list, watch, update, patch, delete)
- Чтение AddonTemplate (get, list, watch)
- Чтение Secrets (для kubeconfig доступа к infra-кластерам)
- Управление ресурсами Addon и AddonValue **в удалённых кластерах** (через kubeconfig)
- Обновление status subresource для AddonClaim
- Events (create, patch)
- Координация leader election (leases)

### Вебхуки

addonclaim-controller обслуживает валидирующие вебхуки для:

- **AddonClaim** — валидация `credentialRef`, `templateRef`, взаимоисключение `values`/`valuesString`
- **AddonTemplate** — валидация Go template синтаксиса в `spec.template`

> **Важно:** Вебхуки AddonClaim и AddonTemplate должны указывать на Service addonclaim-controller, а **не** на Service основного addons-operator.

---

## Удаление

Удаление контроллера:

```bash
make undeploy
```

Удаление CRD (это удалит все Addon ресурсы):

```bash
make uninstall
```

## Следующие шаги

- [Развёртывание аддонов](deploying-addons.md) — создание первого аддона
- [Мультикластерное развёртывание](multi-cluster.md) — AddonClaim и AddonTemplate
- [Быстрый старт](../getting-started.md) — краткое руководство
