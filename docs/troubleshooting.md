# Устранение неполадок

Это руководство поможет диагностировать и решить типичные проблемы с Addon Operator.

## Проверка статуса

### Статус Addon

```bash
kubectl get addon <name> -o yaml
```

Ключевые поля для проверки:
- `status.conditions` — детальный статус conditions
- `status.applicationRef` — ссылка на Argo CD Application

### Справочник по Conditions

| Condition | Status | Reason | Значение |
|-----------|--------|--------|----------|
| `Ready` | True | FullyReconciled | Аддон полностью работает |
| `Ready` | False | DependenciesNotMet | Заблокирован initDependencies |
| `Ready` | False | ValuesNotResolved | Ошибка разрешения values |
| `Progressing` | True | Reconciling | Выполняется reconciliation |
| `Degraded` | True | * | Произошла ошибка |
| `ApplicationCreated` | True | ApplicationCreated | Argo CD Application существует |
| `ValuesResolved` | True | ValuesResolved | Все values обработаны |
| `ValuesResolved` | False | ValueSourceError | Отсутствует Secret/ConfigMap |
| `ValuesResolved` | False | TemplateError | Ошибка рендеринга шаблона |
| `DependenciesMet` | True | AllDependenciesMet | Все зависимости готовы |
| `DependenciesMet` | False | WaitingForDependency | Зависимость не готова |
| `Synced` | True | Synced | Application синхронизирован |
| `Healthy` | True | Healthy | Application здоров |

## Типичные проблемы

### 1. WaitingForDependencies

**Симптом:**
```yaml
status:
  conditions:
    - type: Ready
      status: "False"
      reason: WaitingForDependencies
    - type: DependenciesMet
      status: "False"
      reason: WaitingForDependencies
      message: "Waiting for dependency: cert-manager"
```

**Причина:** Addon имеет `initDependencies`, но criteria зависимости не выполнены.

**Решение:**
1. Проверьте статус зависимого Addon:
   ```bash
   kubectl get addon cert-manager -o yaml
   ```
2. Проверьте, что criteria путь существует:
   ```bash
   kubectl get addon cert-manager -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'
   ```
3. Дождитесь готовности зависимости или удалите зависимость, если не нужна.

### 2. ValueSourceError

**Симптом:**
```yaml
status:
  conditions:
    - type: ValuesResolved
      status: "False"
      reason: ValueSourceError
      message: "Secret default/database-credentials not found"
```

**Причина:** valuesSources ссылается на Secret или ConfigMap, который не существует.

**Решение:**
1. Создайте отсутствующий ресурс:
   ```bash
   kubectl create secret generic database-credentials \
     --from-literal=username=admin \
     --from-literal=password=secret
   ```
2. Или обновите Addon, удалив ссылку valuesSources.

### 3. TemplateError

**Симптом:**
```yaml
status:
  conditions:
    - type: ValuesResolved
      status: "False"
      reason: TemplateError
      message: "template error: .Variables.cluster_name undefined"
```

**Причина:** Шаблон ссылается на переменную, не определённую в `variables` Addon.

**Решение:**
1. Добавьте отсутствующую переменную в Addon:
   ```yaml
   spec:
     variables:
       cluster_name: production
   ```
2. Или используйте функцию `default` в шаблоне:
   ```yaml
   cluster: "{{ .Variables.cluster_name | default \"default-cluster\" }}"
   ```

### 4. Application не создан

**Симптом:** Addon существует, но Application в namespace argocd нет.

**Причины:**
1. Ожидание зависимостей
2. Ошибка разрешения values
3. Контроллер не запущен

**Решение:**
1. Проверьте conditions Addon на конкретную ошибку
2. Убедитесь, что контроллер работает:
   ```bash
   kubectl get pods -n addon-operator-system
   ```
3. Проверьте логи контроллера:
   ```bash
   kubectl logs -n addon-operator-system -l app=addon-controller
   ```

### 5. Правило AddonPhase не совпадает

**Симптом:**
```yaml
# Статус AddonPhase
status:
  ruleStatuses:
    - name: certificates
      matched: false
      message: "Criterion failed: source Addon cert-manager not found"
```

**Причина:** Исходный ресурс не существует или criteria не выполнены.

**Решение:**
1. Проверьте, что исходный ресурс существует:
   ```bash
   kubectl get addon cert-manager
   ```
2. Проверьте правильность JSONPath:
   ```bash
   kubectl get addon cert-manager -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'
   ```
3. Убедитесь, что ожидаемое значение совпадает:
   ```yaml
   criteria:
     - jsonPath: $.status.conditions[0].status
       operator: Equal
       value: "True"  # Должно точно совпадать
   ```

### 6. Правило AddonPhase не деактивируется (latched)

**Симптом:**
```yaml
status:
  ruleStatuses:
    - name: enable-tls
      matched: true
      latched: true
      message: "All conditions satisfied"
```

Правило продолжает совпадать, хотя исходный ресурс больше не удовлетворяет criteria.

**Причина:** Criteria с `keep: true` (по умолчанию) фиксируются при первом совпадении и больше не перевычисляются. Это защита от каскадных сбоев.

**Решение:**

Если нужно сбросить фиксацию:

1. Удалите и пересоздайте AddonPhase:
   ```bash
   kubectl delete addonphase my-app
   kubectl apply -f addonphase.yaml
   ```

2. Или переименуйте правило в spec (фиксация привязана к имени):
   ```yaml
   # Было: name: enable-tls
   name: enable-tls-v2
   ```

Если правило должно перевычисляться каждый цикл, используйте `keep: false`:
```yaml
criteria:
  - jsonPath: $.status.conditions[0].status
    operator: Equal
    value: "True"
    keep: false  # перевычисляется каждый цикл
```

Подробнее: [Фиксация правил (Latching)](user-guide/rule-latching.md).

### 7. Невозможно изменить keep на существующем AddonPhase

**Симптом:**
```
Error: admission webhook "vaddonphase-v1alpha1.kb.io" denied the request:
spec.rules[enable-tls].criteria[0]: keep value is immutable (was true, got false)
```

**Причина:** Поле `keep` является неизменяемым после создания для защиты целостности фиксации.

**Решение:**

Удалите AddonPhase и создайте заново с нужным значением `keep`:
```bash
kubectl delete addonphase my-app
# Отредактируйте YAML с нужным keep: false
kubectl apply -f addonphase.yaml
```

### 8. Values не объединяются правильно

**Симптом:** Итоговые values не включают ожидаемые values из некоторых AddonValue.

**Причины:**
1. Метки не соответствуют селектору
2. Проблема с порядком приоритетов

**Решение:**
1. Проверьте метки AddonValue:
   ```bash
   kubectl get addonvalue -l addons.in-cloud.io/addon=cilium
   ```
2. Проверьте, что каждый AddonValue соответствует селектору:
   ```yaml
   # Селектор Addon
   valuesSelectors:
     - matchLabels:
         addons.in-cloud.io/addon: cilium
         addons.in-cloud.io/layer: defaults

   # AddonValue должен иметь ВСЕ эти метки
   metadata:
     labels:
       addons.in-cloud.io/addon: cilium
       addons.in-cloud.io/layer: defaults
   ```
3. Помните: больший приоритет (99) перезаписывает меньший (0).

### 9. Application Out of Sync

**Симптом:** Argo CD Application показывает статус OutOfSync.

**Причина:** Это нормальное поведение Argo CD — Application нужно синхронизировать.

**Решение:**
1. Проверьте статус синхронизации Application:
   ```bash
   kubectl get application -n argocd <name>
   ```
2. Синхронизируйте вручную при необходимости:
   ```bash
   argocd app sync <name>
   ```
3. Проверьте события Application на ошибки синхронизации:
   ```bash
   kubectl describe application -n argocd <name>
   ```

### 10. Автоматический retry при ошибках

**Поведение:** Addon в состоянии `Degraded` автоматически повторяет reconciliation каждые 60 секунд.

Это означает:
- Не нужно вручную перезапускать контроллер при временных ошибках
- Если проблема устранена (например, создан отсутствующий Secret), Addon восстановится автоматически

### 11. valuesSources на недоступный CRD

**Симптом:**
```
INFO  pending watch still unavailable  {"gvk": "cert-manager.io/v1/Certificate"}
```

**Причина:** valuesSources ссылается на CRD, который ещё не установлен.

**Поведение:**
- Контроллер ставит watch в режим "pending"
- При каждом reconcile проверяет доступность CRD
- Когда CRD появится — watch активируется автоматически
- Событие `WatchActivated` записывается в Events Addon

**Решение:** Установите CRD (например, cert-manager) — Addon автоматически начнёт отслеживать ресурсы.

### 11a. Addon долго не удаляется

**Симптом:** Addon находится в состоянии `Terminating` длительное время.

**Причина:** У ArgoCD Application установлен финализатор `resources-finalizer.argocd.argoproj.io` (через `spec.finalizer: true`). ArgoCD удаляет все managed-ресурсы перед удалением Application, а контроллер Addon ждёт завершения этого процесса.

**Диагностика:**
```bash
# Проверить, существует ли Application
kubectl get application -n argocd <addon-name>

# Посмотреть финализаторы Application
kubectl get application -n argocd <addon-name> -o jsonpath='{.metadata.finalizers}'

# Логи контроллера
kubectl logs -n addon-operator-system -l app=addon-controller | grep "Waiting for ArgoCD Application"
```

**Решение:**
1. Подождите — ArgoCD удаляет ресурсы, это нормальный процесс
2. Если Application зависла, проверьте логи ArgoCD:
   ```bash
   kubectl logs -n argocd -l app.kubernetes.io/name=argocd-application-controller
   ```
3. В крайнем случае, удалите финализатор ArgoCD Application вручную:
   ```bash
   kubectl patch application -n argocd <name> --type merge \
     -p '{"metadata":{"finalizers":null}}'
   ```

### 11b. Невозможно изменить name или cluster в AddonClaim

**Симптом:**
```
Error: admission webhook denied the request: name is immutable
```

**Причина:** Поля `name` и `cluster` в AddonClaim неизменяемы после создания.

**Решение:** Удалите AddonClaim и создайте новый с нужными значениями:
```bash
kubectl delete addonclaim <name> -n <namespace>
kubectl apply -f addonclaim-new.yaml
```

## AddonClaim

### 12. SecretNotFound

**Симптом:**
```yaml
status:
  conditions:
    - type: RemoteConnected
      status: "False"
      reason: SecretNotFound
      message: 'Secret "infra-kubeconfig" not found'
    - type: Degraded
      status: "True"
      reason: SecretNotFound
```

**Причина:** Secret с kubeconfig не найден.

**Решение:**
1. Secret должен быть в том же namespace, что и AddonClaim:
   ```bash
   kubectl get secret infra-kubeconfig -n <addonclaim-namespace>
   ```
2. Создайте Secret, если отсутствует:
   ```bash
   kubectl create secret generic infra-kubeconfig \
     --namespace=<addonclaim-namespace> \
     --from-file=value=/path/to/kubeconfig
   ```

### 13. SecretInvalid

**Симптом:**
```yaml
status:
  conditions:
    - type: RemoteConnected
      status: "False"
      reason: SecretInvalid
      message: 'Secret "infra-kubeconfig" missing key "value"'
```

**Причина:** Secret не содержит ключ `value` с kubeconfig.

**Решение:**
Secret должен содержать ключ `value`:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: infra-kubeconfig
type: Opaque
data:
  value: <base64-encoded kubeconfig>
```

### 14. TemplateNotFound

**Симптом:**
```yaml
status:
  conditions:
    - type: TemplateRendered
      status: "False"
      reason: TemplateNotFound
      message: 'AddonTemplate "cilium-v1.17.4" not found'
```

**Причина:** AddonTemplate не существует.

**Решение:**
1. AddonTemplate — cluster-scoped ресурс, namespace не нужен:
   ```bash
   kubectl get addontemplate cilium-v1.17.4
   ```
2. Создайте AddonTemplate, если отсутствует.

### 15. TemplateRenderFailed

**Симптом:**
```yaml
status:
  conditions:
    - type: TemplateRendered
      status: "False"
      reason: TemplateRenderFailed
      message: "execute template: template: addon-template:3:15: ..."
```

**Причина:** Ошибка при рендеринге Go template.

**Решение:**
1. Проверьте синтаксис шаблона:
   ```bash
   kubectl get addontemplate <name> -o jsonpath='{.spec.template}'
   ```
2. Убедитесь, что используемые переменные существуют в контексте (`.Values.spec.name`, `.Values.spec.version` и т.д.)
3. При использовании `missingkey=error` все обращения к несуществующим ключам вызовут ошибку.

### 16. RemoteClientFailed

**Симптом:**
```yaml
status:
  conditions:
    - type: RemoteConnected
      status: "False"
      reason: RemoteClientFailed
```

**Причина:** Kubeconfig в Secret невалиден или infra-кластер недоступен.

**Решение:**
1. Проверьте, что kubeconfig валиден:
   ```bash
   kubectl get secret infra-kubeconfig -n <ns> -o jsonpath='{.data.value}' | base64 -d > /tmp/kc.yaml
   kubectl --kubeconfig=/tmp/kc.yaml cluster-info
   ```
2. Проверьте сетевую доступность infra-кластера из system-кластера.

### 17. RemoteOperationFailed

**Симптом:**
```yaml
status:
  conditions:
    - type: AddonSynced
      status: "False"
      reason: RemoteOperationFailed
```

**Причина:** Не удалось создать/обновить ресурсы в infra-кластере.

**Решение:**
1. Убедитесь, что CRD Addon и AddonValue установлены в infra-кластере:
   ```bash
   kubectl --kubeconfig=/path/to/infra-kubeconfig get crd addons.addons.in-cloud.io
   kubectl --kubeconfig=/path/to/infra-kubeconfig get crd addonvalues.addons.in-cloud.io
   ```
2. Проверьте RBAC — ServiceAccount в kubeconfig должен иметь права на создание Addon и AddonValue.

### 18. AddonClaim stuck в Progressing

**Симптом:**
```yaml
status:
  ready: false
  conditions:
    - type: Ready
      status: "False"
      reason: AddonNotReady
    - type: Progressing
      status: "True"
      reason: Reconciling
      message: "Waiting for remote Addon to become ready"
    - type: AddonSynced
      status: "True"
      reason: Synced
```

**Причина:** Addon и AddonValue успешно синхронизированы, но удалённый Addon не переходит в Ready.

**Решение:**
1. Проверьте статус Addon в infra-кластере:
   ```bash
   kubectl --kubeconfig=/path/to/infra-kubeconfig get addon <name> -o yaml
   ```
2. Убедитесь, что addons-operator запущен в infra-кластере.
3. Проверьте, что Argo CD работает в infra-кластере.
4. Проверьте логи addonclaim-controller:
   ```bash
   kubectl logs -n addon-operator-system -l app=addonclaim-controller
   ```

## Команды отладки

### Просмотр всех Addon

```bash
kubectl get addons -A -o wide
```

### Просмотр всех AddonValue для Addon

```bash
kubectl get addonvalue -l addons.in-cloud.io/addon=<name>
```

### Просмотр статуса AddonPhase

```bash
kubectl get addonphase <name> -o yaml
```

### Проверка логов контроллера

```bash
kubectl logs -n addon-operator-system -l app=addon-controller -f
```

### Проверка Argo CD Application

```bash
kubectl get application -n argocd <name> -o yaml
argocd app get <name>
```

### Просмотр статуса AddonClaim

```bash
kubectl get addonclaim -n <namespace>
kubectl get addonclaim <name> -n <namespace> -o yaml
```

### Проверка удалённого Addon из AddonClaim

```bash
# Получить имя аддона
kubectl get addonclaim <name> -n <namespace> -o jsonpath='{.spec.name}'

# Проверить Addon в infra-кластере
kubectl --kubeconfig=/path/to/infra-kubeconfig get addon <addon-name> -o yaml
```

### Трассировка разрешения Values

```bash
# Получить хеш values
kubectl get addon <name> -o jsonpath='{.status.valuesHash}'

# Сравнить с values Application
kubectl get application -n argocd <name> -o jsonpath='{.spec.source.helm.values}'
```

## Настройка логирования

Контроллер использует структурированное логирование с уровнями детализации.

### Уровни логирования

| Уровень | Флаг | Что логируется |
|---------|------|----------------|
| 0 (default) | `-v=0` | Важные события: создание/удаление ресурсов, ошибки |
| 1 (debug) | `-v=1` | + каждый reconcile, промежуточные шаги |
| 2 (verbose) | `-v=2` | + детали алгоритмов, полные объекты |

### Примеры логов по уровням

**Уровень 0 (production):**
```
INFO  Creating Argo CD Application  {"name": "prometheus", "namespace": "argocd"}
INFO  watch started  {"gvk": "v1/Secret"}
ERROR Failed to extract values sources  {"addon": "myapp", "reason": "Secret not found"}
```

**Уровень 1 (debug):**
```
INFO  Reconciling Addon  {"chart": "prometheus", "version": "55.5.0"}
INFO  Waiting for dependencies
INFO  pending watch still unavailable  {"gvk": "cert-manager.io/v1/Certificate"}
```

### Включение debug-логирования

**При запуске контроллера:**
```bash
# В Deployment
args:
  - --zap-log-level=1
```

**Через Kustomize:**
```yaml
# config/manager/manager.yaml
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --zap-log-level=1  # debug mode
```

**Локально:**
```bash
make run ARGS="--zap-log-level=1"
```

### Рекомендации

| Окружение | Уровень | Причина |
|-----------|---------|---------|
| Production | 0 | Минимум шума, только важные события |
| Staging | 1 | Видно каждый reconcile для отладки |
| Development | 2 | Полная детализация |

### Фильтрация логов по компоненту

```bash
# Только логи addon-controller
kubectl logs -n addon-operator-system -l app=addon-controller | grep "addon-controller"

# Только логи addonclaim-controller
kubectl logs -n addon-operator-system -l app=addonclaim-controller

# Только логи dynamic watches
kubectl logs -n addon-operator-system -l app=addon-controller | grep "dynamicwatch"

# Только ошибки
kubectl logs -n addon-operator-system -l app=addon-controller | grep "ERROR"
```

## Получение помощи

Если не удаётся решить проблему:

1. Соберите отладочную информацию:
   ```bash
   kubectl get addon <name> -o yaml > addon.yaml
   kubectl get addonvalue -l addons.in-cloud.io/addon=<name> -o yaml > values.yaml
   kubectl logs -n addon-operator-system -l app=addon-controller --tail=100 > logs.txt
   ```

2. Откройте issue: https://github.com/PRO-Robotech/addons-operator/issues
