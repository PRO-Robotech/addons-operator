# AddonTemplate

Ресурс AddonTemplate определяет переиспользуемый шаблон для генерации Addon из AddonClaim.

## Обзор

AddonTemplate:
- Содержит Go template, который рендерится в YAML манифест Addon
- Является **cluster-scoped** ресурсом (доступен из любого namespace)
- Используется AddonClaim через `templateRef`
- Валидируется webhook'ом при создании и обновлении

## Поля Spec

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `template` | string | Да | Go template, рендерящийся в YAML манифест Addon |

## Контекст шаблона

Шаблон получает AddonClaim как контекст. Доступны два способа доступа к данным:

### `.Vars` — быстрый доступ к переменным (рекомендуется)

| Путь | Описание | Пример |
|------|----------|--------|
| `.Vars.<key>` | Любая переменная из `spec.variables` | `.Vars.name` → `cilium` |

### `.Values` — полный доступ к AddonClaim

| Путь | Описание | Пример |
|------|----------|--------|
| `.Values.spec.addon.name` | Имя Addon в удалённом кластере | `cilium` |
| `.Values.spec.variables.<key>` | Переменные (полный путь) | `v1.17.4` |
| `.Values.spec.credentialRef.name` | Имя Secret | `infra-kubeconfig` |
| `.Values.spec.templateRef.name` | Имя шаблона | `cilium-v1.17.4` |
| `.Values.metadata.name` | Имя AddonClaim | `cilium` |
| `.Values.metadata.namespace` | Namespace AddonClaim | `tenant-a` |

> **Важно:** Поле `metadata.name` в отрендеренном шаблоне всегда переопределяется значением `spec.addon.name` из AddonClaim. Шаблон определяет спецификацию Addon (chart, repo, backend и т.д.), а идентификация задаётся явно.

Для переменных с нестандартными символами в имени используйте `index`:
```
{{ index .Values.spec.variables "my-key" }}
```

## Доступные функции

В шаблонах доступны все функции [Sprig v3](https://masterminds.github.io/sprig/):

| Категория | Примеры функций |
|-----------|----------------|
| Строки | `trim`, `upper`, `lower`, `replace`, `contains`, `quote` |
| Условия | `default`, `empty`, `coalesce`, `ternary` |
| Списки | `list`, `first`, `last`, `join`, `has` |
| Словари | `dict`, `get`, `set`, `hasKey`, `merge` |
| Кодирование | `b64enc`, `b64dec`, `toJson`, `toYaml` |
| Crypto | `sha256sum`, `genPrivateKey` |

## Примеры

### Базовый шаблон

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonTemplate
metadata:
  name: cilium-v1.17.4
spec:
  template: |
    apiVersion: addons.in-cloud.io/v1alpha1
    kind: Addon
    metadata:
      name: placeholder  # переопределяется значением spec.addon.name из AddonClaim
    spec:
      path: "helm-chart-sources/{{ .Vars.name }}"
      pluginName: helm-with-values
      repoURL: "https://github.com/org/helm-charts"
      version: "{{ .Vars.version }}"
      releaseName: {{ .Vars.name }}
      targetCluster: "{{ .Vars.cluster }}"
      targetNamespace: "{{ .Vars.name }}-system"
      backend:
        type: argocd
        namespace: argocd
      variables:
        cluster_name: "{{ .Vars.cluster }}"
```

> **Примечание:** Поле `metadata.name` в шаблоне всегда переопределяется значением `spec.addon.name` из AddonClaim. Шаблон определяет спецификацию Addon, а идентификация (имя) задаётся явно через AddonClaim.

### С условной логикой

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonTemplate
metadata:
  name: monitoring-v1
spec:
  template: |
    apiVersion: addons.in-cloud.io/v1alpha1
    kind: Addon
    metadata:
      name: placeholder  # переопределяется spec.addon.name
    spec:
      chart: kube-prometheus-stack
      repoURL: https://prometheus-community.github.io/helm-charts
      version: "{{ .Vars.version }}"
      targetCluster: "{{ .Vars.cluster }}"
      targetNamespace: monitoring
      backend:
        type: argocd
        namespace: argocd
        {{- if eq .Values.metadata.namespace "production" }}
        project: production
        {{- else }}
        project: default
        {{- end }}
```

### С функциями Sprig

```yaml
apiVersion: addons.in-cloud.io/v1alpha1
kind: AddonTemplate
metadata:
  name: app-v2
spec:
  template: |
    apiVersion: addons.in-cloud.io/v1alpha1
    kind: Addon
    metadata:
      name: placeholder  # переопределяется spec.addon.name
    spec:
      chart: {{ .Vars.name }}
      repoURL: https://charts.example.com
      version: {{ .Vars.version | trimPrefix "v" | quote }}
      targetCluster: {{ .Vars.cluster | quote }}
      targetNamespace: {{ printf "app-%s" .Vars.name }}
      backend:
        type: argocd
        namespace: argocd
```

## Валидация

При создании и обновлении AddonTemplate webhook проверяет:
- Поле `template` не пустое
- Шаблон корректно парсится как Go template (синтаксис `{{ }}`, правильные имена функций)

Ошибки синтаксиса шаблона отклоняются немедленно:

```
Error: admission webhook "vaddontemplate-v1alpha1.kb.io" denied the request:
invalid template: parse template: template: addon-template:5: function "unknownFunc" not defined
```

## Соглашения по меткам

Рекомендуется использовать метки для организации шаблонов:

| Метка | Назначение | Пример |
|-------|------------|--------|
| `name.addons.in-cloud.io` | Имя аддона | `cilium` |
| `version.addons.in-cloud.io` | Версия аддона | `v1.17.4` |

```yaml
metadata:
  name: cilium-v1.17.4
  labels:
    name.addons.in-cloud.io: cilium
    version.addons.in-cloud.io: v1.17.4
```

## Связанные ресурсы

- [AddonClaim](addon-claim.md) — запрос на развёртывание, использующий шаблон
- [Addon](addon.md) — результат рендеринга шаблона
- [Мультикластерное развёртывание](../user-guide/multi-cluster.md) — пошаговое руководство
