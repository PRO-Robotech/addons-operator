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
- [Быстрый старт](../getting-started.md) — краткое руководство
