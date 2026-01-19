# Обновления

Это руководство описывает обновление версий аддонов и управление изменениями.

## Обновление версии Addon

### Метод 1: kubectl patch

```bash
kubectl patch addon prometheus --type=merge -p '{"spec":{"version":"55.6.0"}}'
```

### Метод 2: kubectl edit

```bash
kubectl edit addon prometheus
# Измените spec.version
```

### Метод 3: Применение обновлённого манифеста

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: prometheus
spec:
  version: "55.6.0"  # Обновлённая версия
  # ... остальная спецификация
```

```bash
kubectl apply -f prometheus-addon.yaml
```

## Процесс обновления

1. Изменяется spec.version в Addon
2. Контроллер обновляет Argo CD Application
3. Argo CD синхронизирует новую версию
4. Status conditions отражают прогресс синхронизации

## Мониторинг обновления

Наблюдение за процессом обновления:

```bash
# Наблюдение за статусом аддона
kubectl get addon prometheus -w

# Проверка conditions
kubectl get addon prometheus -o jsonpath='{.status.conditions[*].type}'

# Просмотр статуса синхронизации Argo CD Application
kubectl get application -n argocd prometheus -o jsonpath='{.status.sync.status}'
```

## Status Conditions при обновлении

| Фаза | Conditions |
|------|------------|
| Начало | `Progressing=True`, `Synced=False` |
| Синхронизация | `Progressing=True`, `Synced=False` |
| Завершено | `Ready=True`, `Synced=True`, `Healthy=True` |
| Ошибка | `Ready=False`, `Degraded=True` |

## Откат

### Через Argo CD

Argo CD хранит историю ревизий:

```bash
# Список ревизий
argocd app history prometheus

# Откат к предыдущей ревизии
argocd app rollback prometheus 2
```

### Через версию в Addon

Возврат к предыдущей версии в спецификации Addon:

```bash
kubectl patch addon prometheus --type=merge -p '{"spec":{"version":"55.5.0"}}'
```

## Безопасные практики обновления

### 1. Проверка Release Notes

Перед обновлением изучите release notes chart'а на предмет breaking changes.

### 2. Тестирование в Staging

Сначала разверните в staging окружение:

```yaml
# staging-prometheus.yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: prometheus
  namespace: staging
spec:
  version: "55.6.0"
```

### 3. Постепенный rollout

Для мульти-кластерных развёртываний обновляйте кластеры поэтапно.

### 4. Мониторинг здоровья

```bash
# Наблюдение за статусом здоровья
kubectl get addon prometheus -o jsonpath='{.status.conditions[?(@.type=="Healthy")]}' -w
```

## Обработка ошибок обновления

### Проверка Conditions Addon

```bash
kubectl get addon prometheus -o yaml
```

Обратите внимание на:
- Condition `Degraded` с сообщением об ошибке
- `Synced=False` с ошибкой синхронизации
- `Healthy=False` с проблемами здоровья

### Проверка Argo CD Application

```bash
kubectl get application -n argocd prometheus -o yaml
```

Обратите внимание на:
- `status.sync.status` — состояние синхронизации
- `status.health.status` — состояние здоровья
- `status.conditions` — детальные ошибки

### Частые проблемы

| Проблема | Симптом | Решение |
|----------|---------|---------|
| Невалидные values | `Degraded=True` | Исправьте содержимое AddonValue |
| Отсутствует зависимость | `DependenciesMet=False` | Проверьте аддон-зависимость |
| Chart не найден | `Synced=False` | Проверьте repoURL и version |
| Конфликт ресурсов | `Healthy=False` | Проверьте владение ресурсами |

## Ограничения версий

Используйте семантическое версионирование в поле version:

```yaml
spec:
  version: "55.6.0"      # Точная версия
  version: "55.6.*"      # Wildcard для патчей (если поддерживается репозиторием)
```

## Автоматические обновления

Для автоматического обновления версий рассмотрите:

1. **Renovate/Dependabot** — обновление версии в GitOps репозитории
2. **Argo CD Image Updater** — для container images
3. **Кастомный контроллер** — отслеживание новых версий chart'ов

## Следующие шаги

- [Мониторинг](monitoring.md) — мониторинг здоровья аддонов
- [Устранение неполадок](../troubleshooting.md) — отладка частых проблем
