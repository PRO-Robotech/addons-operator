# История изменений

Все значимые изменения в Addon Operator документируются в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/),
проект придерживается [Семантического версионирования](https://semver.org/lang/ru/).

## [Unreleased]

### Добавлено

- **AddonClaim CRD** — мультикластерное управление аддонами
  - Namespaced ресурс для запроса развёртывания аддона в удалённом infra-кластере
  - Поддержка values через JSON объект (`values`) или YAML строку (`valuesString`)
  - Зависимости через поле `dependency` (устанавливает аннотацию на Addon)
  - Status conditions: TemplateRendered, RemoteConnected, AddonSynced, Ready, Progressing, Degraded
  - Зеркалирование статуса удалённого Addon в `status.remoteAddonStatus`

- **AddonTemplate CRD** — шаблоны для генерации Addon из AddonClaim
  - Cluster-scoped ресурс с Go template для рендеринга Addon YAML
  - Контекст шаблона: `.Values.spec.*`, `.Values.metadata.*`
  - Sprig v3 функции доступны в шаблонах
  - Валидация синтаксиса шаблона через webhook

- **addonclaim-controller** — отдельный бинарник для мультикластерного управления
  - Флаг `--polling-interval` для настройки интервала опроса удалённого кластера (по умолчанию 15s)
  - Кэширование remote client'ов по kubeconfig Secret
  - Автоматическая очистка удалённых ресурсов при удалении AddonClaim (finalizer)

- **Dockerfile.addonclaim** — отдельный Dockerfile для сборки addonclaim-controller

## [0.2.0] - 2026-02-18

### Добавлено

- **Config Management Plugin** — новое поле `spec.pluginName` в Addon
  - Позволяет использовать ArgoCD Config Management Plugin вместо встроенного Helm
  - Values передаются через переменную окружения `HELM_VALUES` (base64-encoded YAML)

- **Переопределение Release Name** — новое поле `spec.releaseName` в Addon
  - В Helm режиме устанавливается в `source.helm.releaseName`
  - В Plugin режиме передаётся как переменная окружения `RELEASE_NAME`

- **Каскадное удаление ресурсов** — новое поле `spec.finalizer` в Addon
  - При `finalizer: true` на Argo CD Application устанавливается финалайзер `resources-finalizer.argocd.argoproj.io`
  - При удалении Application Argo CD удаляет все созданные ресурсы перед удалением самого объекта
  - По умолчанию поведение не изменилось — без `finalizer` удаляется только объект Application

- **RFC 9535 JSONPath** — миграция criteria engine на RFC 9535 JSONPath синтаксис (`$.status.conditions[?@.type=='Ready'].status`). Все JSONPath теперь начинаются с `$`.

- **Фиксация правил (Rule Latching)** — поле `keep` в Criterion:
  - `keep: true` (по умолчанию) — criterion фиксируется после первого совпадения
  - `keep: false` — criterion перевычисляется каждый цикл
  - Поле `latched` в `RuleStatus` показывает зафиксированные правила
  - `keep` неизменяем после создания (webhook)

- **Стабилизация values** — при первом создании Application, контроллер дожидается стабилизации хеша values (два последовательных reconcile с одинаковым хешем). Предотвращает race condition при одновременном создании Addon и зависимых ресурсов.

- **Пауза reconciliation** — аннотация `addons.in-cloud.io/paused=true` останавливает reconcile Addon для ручной отладки Application в ArgoCD.

- **Статус первого развёртывания** — новое поле `status.deployed` в Addon
  - Устанавливается в `true` при первом успешном развёртывании (Synced + Healthy)
  - Никогда не сбрасывается обратно в `false` (latching)
  - Позволяет отличить «никогда не был развёрнут» от «был развёрнут, но сейчас нездоров»
  - Доступно через `kubectl get addon` (колонка Deployed)
  - Можно использовать в criteria: `$.status.deployed`

### Изменено

- **AddonPhase webhook** — убрана проверка существования Addon при создании AddonPhase. Теперь AddonPhase можно создавать до Addon (use case: предварительная заготовка через Helm chart). Контроллер ставит condition `TargetAddonNotFound` и ждёт появления Addon.

- **JSONPath синтаксис** — изменён с простого пути на RFC 9535 (с `$` префиксом). Требуется обновление существующих AddonPhase/Addon ресурсов при миграции.

### Исправлено

- **Цикл обновления conditions** — контроллеры Addon и AddonPhase могли бесконечно обновлять status из-за промежуточного изменения `LastTransitionTime` в conditions. Исправлено удалением избыточного `SetProgressing` в начале reconcile.

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

[0.2.0]: https://github.com/PRO-Robotech/addons-operator/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/PRO-Robotech/addons-operator/releases/tag/v0.1.0
