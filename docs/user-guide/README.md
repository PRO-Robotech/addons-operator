# Руководство пользователя

Этот раздел содержит пошаговые инструкции по управлению аддонами с помощью Addon Operator.

## Руководства

| Руководство | Описание |
|-------------|----------|
| [Установка](installation.md) | Установка оператора в кластер |
| [Развёртывание аддонов](deploying-addons.md) | Создание и управление Addon ресурсами |
| [Управление values](managing-values.md) | Работа с AddonValue и источниками значений |
| [Условное развёртывание](conditional-deployment.md) | Использование AddonPhase для условной конфигурации |
| [Зависимости](dependencies.md) | Настройка зависимостей между аддонами |
| [Обновления](upgrades.md) | Обновление версий аддонов |
| [Мониторинг](monitoring.md) | Мониторинг здоровья аддонов и отладка |

## Краткий справочник

### Частые команды kubectl

```bash
# Список всех аддонов
kubectl get addons -A

# Проверка статуса аддона
kubectl get addon <имя> -o wide

# Просмотр conditions аддона
kubectl get addon <имя> -o jsonpath='{.status.conditions[*].type}'

# Список addon values
kubectl get addonvalues -l addons.in-cloud.io/addon=<имя-аддона>

# Проверка итоговых values (в Argo CD Application)
kubectl get application -n argocd <имя> -o jsonpath='{.spec.source.helm.values}'

# Просмотр Argo CD Application
kubectl get application -n argocd <имя-аддона>
```

### Status Conditions

| Condition | True | False |
|-----------|------|-------|
| Ready | Аддон полностью работает | Аддон имеет проблемы |
| DependenciesMet | Все зависимости готовы | Ожидание зависимостей |
| ValuesResolved | Values агрегированы | Ошибка агрегации values |
| ApplicationCreated | Argo CD app существует | App не создан |
| Synced | App синхронизирован | Ожидание синхронизации |
| Healthy | App здоров | App нездоров |

## Рабочий процесс

```
1. Создать AddonValues (конфигурация)
   ↓
2. Создать Addon (определение развёртывания)
   ↓
3. (Опционально) Создать AddonPhase (условные values)
   ↓
4. Мониторить status conditions
```
