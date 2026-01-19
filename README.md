# Addon Operator

Kubernetes Operator для декларативного управления аддонами кластера через интеграцию с Argo CD.

## Что это?

Addon Operator автоматизирует установку, обновление и управление жизненным циклом дополнительных компонентов (аддонов) в Kubernetes кластерах. Вместо ручного управления Helm релизами, вы описываете желаемое состояние через Custom Resources, а оператор обеспечивает его достижение.

## Ключевые возможности

- **Декларативное управление** — аддоны описываются как Kubernetes ресурсы
- **Агрегация конфигурации** — сборка Helm values из нескольких источников с приоритетами
- **Условная конфигурация** — активация настроек в зависимости от состояния кластера
- **Управление зависимостями** — контроль порядка развёртывания аддонов
- **GitOps интеграция** — Argo CD как backend для развёртывания

## Архитектура

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

## Быстрый старт

### Установка CRD

```bash
make install
```

### Развёртывание оператора

```bash
make deploy IMG=<your-registry>/addon-operator:latest
```

### Создание первого аддона

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: prometheus
spec:
  chart: kube-prometheus-stack
  repoURL: https://prometheus-community.github.io/helm-charts
  version: "55.6.0"
  targetCluster: in-cluster
  targetNamespace: monitoring
```

```bash
kubectl apply -f prometheus-addon.yaml
```

### Проверка статуса

```bash
kubectl get addon prometheus
```

## Документация

Подробная документация доступна в директории [docs/](docs/):

| Раздел | Описание |
|--------|----------|
| [Быстрый старт](docs/getting-started.md) | Установка и первые шаги |
| [Концепции](docs/concepts/) | Addon, AddonValue, AddonPhase |
| [Руководство](docs/user-guide/) | Пошаговые инструкции |
| [Справочник API](docs/reference/) | Спецификации CRD |
| [Примеры](docs/examples/) | Готовые конфигурации |
| [Устранение неполадок](docs/troubleshooting.md) | Решение проблем |

## Требования

- Kubernetes 1.28+
- Argo CD 2.8+
- Go 1.21+ (для сборки)

## Разработка

```bash
# Запуск локально
make run

# Тесты
make test

# Линтер
make lint

# Генерация CRD манифестов
make manifests
```

## Лицензия

Apache License 2.0. См. [LICENSE](LICENSE).
