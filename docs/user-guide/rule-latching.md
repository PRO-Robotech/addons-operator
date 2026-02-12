# Фиксация правил (Rule Latching)

Это руководство объясняет механизм фиксации (`keep`) criteria в AddonPhase, который защищает от каскадных сбоев при временной недоступности зависимостей.

## Проблема

При обычном вычислении правил каждый criterion проверяется заново на каждом цикле reconcile. Если зависимый аддон (например, cert-manager) временно теряет статус Ready — например, во время обновления — все зависящие от него правила мгновенно деактивируются. Это вызывает:

- Удаление values из конфигурации аддона
- Пересборку Argo CD Application без нужных настроек
- Каскадные сбои зависимых сервисов
- Цикл осцилляции при восстановлении зависимости

## Решение: поле `keep`

Каждый criterion имеет поле `keep` (по умолчанию `true`). Criterion с `keep: true` **фиксируется** при первом совпадении и больше не перевычисляется — считается автоматически выполненным.

```yaml
criteria:
  - source:
      apiVersion: addons.in-cloud.io/v1alpha1
      kind: Addon
      name: cert-manager
    jsonPath: $.status.conditions[?@.type=='Ready'].status
    operator: Equal
    value: "True"
    # keep: true — по умолчанию, можно не указывать
```

После того как cert-manager однажды стал Ready, criterion фиксируется. Даже если cert-manager временно перестанет быть Ready (при обновлении), правило останется активным.

## Как работает

```
Цикл 1: cert-manager Ready=True
  → criterion совпадает
  → правило совпадает
  → Latched=true записывается в status

Цикл 2: cert-manager обновляется, Ready=Unknown
  → criterion с keep=true пропускается (latched)
  → правило по-прежнему совпадает ✓

Цикл 3: cert-manager восстановился, Ready=True
  → criterion с keep=true пропускается (latched)
  → правило по-прежнему совпадает ✓
```

Контроллер хранит состояние фиксации в `status.ruleStatuses[].latched`:

```yaml
status:
  ruleStatuses:
    - name: enable-tls
      matched: true
      latched: true   # keep=true criteria зафиксированы
      message: "All conditions satisfied"
```

## Комбинирование keep: true и keep: false

В одном правиле можно комбинировать фиксированные и динамические criteria:

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonPhase
metadata:
  name: my-app
spec:
  rules:
    - name: ha-with-tls
      criteria:
        # Фиксируется навсегда — cert-manager может временно упасть
        - source:
            apiVersion: addons.in-cloud.io/v1alpha1
            kind: Addon
            name: cert-manager
          jsonPath: $.status.conditions[?@.type=='Ready'].status
          operator: Equal
          value: "True"
          # keep: true (по умолчанию)

        # Перевычисляется каждый цикл — при масштабировании вниз правило деактивируется
        - source:
            apiVersion: apps/v1
            kind: Deployment
            name: my-app
            namespace: default
          jsonPath: $.spec.replicas
          operator: GreaterOrEqual
          value: 3
          keep: false
      selector:
        name: ha-tls-values
        priority: 20
        matchLabels:
          addons.in-cloud.io/addon: my-app
          addons.in-cloud.io/feature.ha-tls: "true"
```

**Поведение:**

| Событие | cert-manager criterion | replicas criterion | Правило |
|---------|----------------------|-------------------|---------|
| Первый match | ✓ совпал → зафиксирован | ✓ совпал (replicas=3) | ✓ active |
| cert-manager обновляется | ✓ пропущен (latched) | ✓ совпал (replicas=3) | ✓ active |
| Масштабирование вниз | ✓ пропущен (latched) | ✗ не совпал (replicas=1) | ✗ inactive |
| Масштабирование обратно | ✓ пропущен (latched) | ✓ совпал (replicas=3) | ✓ active |

## Полностью динамические правила

Если все criteria имеют `keep: false`, правило перевычисляется полностью каждый цикл и никогда не фиксируется:

```yaml
rules:
  - name: when-config-enabled
    criteria:
      - source:
          apiVersion: v1
          kind: ConfigMap
          name: feature-flags
          namespace: kube-system
        jsonPath: $.data.enable_feature_x
        operator: Equal
        value: "true"
        keep: false  # Деактивируется сразу при изменении ConfigMap
    selector:
      name: feature-x
      priority: 30
      matchLabels:
        addons.in-cloud.io/addon: my-app
        addons.in-cloud.io/feature.x: "true"
```

## Сброс фиксации

Зафиксированное правило можно «сбросить» одним из способов:

1. **Удалить и пересоздать AddonPhase** — очищает весь status:
   ```bash
   kubectl delete addonphase my-app
   kubectl apply -f addonphase.yaml
   ```

2. **Переименовать правило** — фиксация привязана к имени правила. Изменение имени создаёт новое правило без фиксации:
   ```yaml
   # Было: name: enable-tls
   # Стало (сброс):
   name: enable-tls-v2
   ```

## Ограничения

### Неизменяемость keep

Поле `keep` нельзя изменить после создания. Webhook отклонит обновление, если эффективное значение `keep` изменилось:

```
Error: spec.rules[enable-tls].criteria[0]: keep value is immutable (was true, got false)
```

Значения `keep: true` и отсутствие поля `keep` считаются эквивалентными — переход между ними разрешён.

### Зависимости (initDependencies)

Поле `keep` работает только в AddonPhase criteria. На `initDependencies` в Addon оно не влияет — у `initDependencies` своя логика блокировки.

## Проверка статуса фиксации

```bash
# Проверить, зафиксировано ли правило
kubectl get addonphase my-app -o jsonpath='{.status.ruleStatuses[?(@.name=="enable-tls")].latched}'
# Вывод: true

# Посмотреть все правила и их статусы
kubectl get addonphase my-app -o jsonpath='{range .status.ruleStatuses[*]}{.name}: matched={.matched}, latched={.latched}{"\n"}{end}'
# Вывод:
# enable-tls: matched=true, latched=true
# when-replicas-high: matched=true, latched=false
```

## Типичные сценарии

### Защита от каскадного сбоя при обновлении зависимости

```yaml
# cert-manager обновляется → TLS values не удаляются
criteria:
  - source:
      apiVersion: addons.in-cloud.io/v1alpha1
      kind: Addon
      name: cert-manager
    jsonPath: $.status.conditions[?@.type=='Ready'].status
    operator: Equal
    value: "True"
    # keep: true (по умолчанию)
```

### Условная HA-конфигурация с динамической проверкой

```yaml
# HA values активны только пока replicas >= 3
criteria:
  - source:
      apiVersion: apps/v1
      kind: Deployment
      name: backend
      namespace: production
    jsonPath: $.spec.replicas
    operator: GreaterOrEqual
    value: 3
    keep: false
```

### Комбинация: зависимость + динамическое условие

```yaml
# TLS фиксируется, но только если Hubble включён (динамически)
criteria:
  - source:
      apiVersion: addons.in-cloud.io/v1alpha1
      kind: Addon
      name: cert-manager
    jsonPath: $.status.conditions[?@.type=='Ready'].status
    operator: Equal
    value: "True"
    # keep: true — фиксируется

  - source:
      apiVersion: v1
      kind: ConfigMap
      name: cluster-config
      namespace: kube-system
    jsonPath: $.data.enable_hubble
    operator: Equal
    value: "true"
    keep: false  # перевычисляется
```

## Следующие шаги

- [Условное развёртывание](conditional-deployment.md) — основы работы с AddonPhase
- [Зависимости](dependencies.md) — блокировка развёртывания через initDependencies
- [Справочник API](../reference/api.md) — описание полей Criterion
- [Устранение неполадок](../troubleshooting.md) — диагностика проблем
