# История изменений

Все значимые изменения в Addon Operator документируются в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/),
проект придерживается [Семантического версионирования](https://semver.org/lang/ru/).

## [Unreleased]

## [0.1.0] - 2026-01-18

Первый релиз Addon Operator.

### Добавлено

- **Addon CRD** — основной ресурс для управления Helm развёртываниями
  - Спецификация chart, репозитория и версии
  - Конфигурация целевого кластера и namespace
  - Агрегация values из AddonValue ресурсов через selectors
  - Управление зависимостями через `initDependencies`
  - Condition-based статус (Ready, Progressing, Degraded, DependenciesMet, ValuesResolved, ApplicationCreated, Synced, Healthy)

- **AddonValue CRD** — хранение фрагментов Helm values
  - Выбор по меткам через Addon valuesSelectors
  - Слияние по приоритету (больший приоритет перезаписывает меньший)
  - Поддержка Go шаблонов с Variables

- **AddonPhase CRD** — условная активация values по правилам
  - Вычисление criteria на основе состояния других ресурсов
  - Динамическая инъекция selectors в статус связанного Addon
  - Поддержка criteria для Addon и внешних Kubernetes ресурсов

- **Argo CD Backend** — интеграция с Argo CD для развёртывания
  - Автоматическая генерация Application ресурсов
  - Конфигурация политики синхронизации
  - Управление metadata для target namespace (`managedNamespaceMetadata`)
  - Игнорирование drift для определённых ресурсов (`ignoreDifferences`)
  - Отслеживание статуса здоровья и синхронизации

- **Value Sources** — извлечение values из внешних ресурсов
  - Поддержка **любых** Kubernetes ресурсов (Secret, ConfigMap, Deployment, Service, CRD и др.)
  - Извлечение значений по JSONPath
  - Автоматическое декодирование Base64 для данных Secret
  - Динамические watches — изменение любого исходного ресурса автоматически триггерит reconcile

- **Движок Criteria** — вычисление условий по состоянию кластера
  - Извлечение значений по JSONPath
  - Операторы сравнения: Equal, NotEqual, In, NotIn
  - Числовые операторы: GreaterThan, GreaterOrEqual, LessThan, LessOrEqual
  - Regex оператор: Matches
  - Операторы существования: Exists, NotExists

- **Condition Manager** — централизованное управление conditions
  - Атомарное обновление conditions
  - Предопределённые типы: Ready, Progressing, Degraded
  - Операционные conditions: DependenciesMet, ValuesResolved, ApplicationCreated, Synced, Healthy

- **Status Translator** — трансляция статуса Argo CD Application
  - Маппинг статуса синхронизации в condition Synced
  - Маппинг статуса здоровья в condition Healthy

- **Validating Webhooks** — валидация CRD ресурсов
  - Проверка корректности spec полей
  - Валидация ссылок и selectors

### Архитектурные решения

- Структура из трёх CRD (Addon, AddonValue, AddonPhase)
- Связь AddonPhase с Addon 1:1 по имени
- Выбор AddonValue по меткам (matchLabels)
- Deep merge values по приоритету
- Argo CD как deployment backend
- JSONPath + operators для criteria
- Go шаблоны для динамических values
- Condition-centric reconciliation

---

[Unreleased]: https://github.com/PRO-Robotech/addons-operator/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/PRO-Robotech/addons-operator/releases/tag/v0.1.0
