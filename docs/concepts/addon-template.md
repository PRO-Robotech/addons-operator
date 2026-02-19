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

Шаблон получает AddonClaim как контекст через `.Values`:

| Путь | Описание | Пример |
|------|----------|--------|
| `.Values.spec.name` | Имя аддона | `cilium` |
| `.Values.spec.version` | Версия аддона | `v1.17.4` |
| `.Values.spec.cluster` | Имя целевого кластера | `client-cluster-01` |
| `.Values.spec.credentialRef.name` | Имя Secret | `infra-kubeconfig` |
| `.Values.spec.templateRef.name` | Имя шаблона | `cilium-v1.17.4` |
| `.Values.metadata.name` | Имя AddonClaim | `cilium` |
| `.Values.metadata.namespace` | Namespace AddonClaim | `tenant-a` |

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
      name: {{ .Values.spec.name }}
    spec:
      path: "helm-chart-sources/{{ .Values.spec.name }}"
      pluginName: helm-with-values
      repoURL: "https://github.com/org/helm-charts"
      version: "{{ .Values.spec.version }}"
      releaseName: {{ .Values.spec.name }}
      targetCluster: "{{ .Values.spec.cluster }}"
      targetNamespace: "{{ .Values.spec.name }}-system"
      backend:
        type: argocd
        namespace: argocd
      variables:
        cluster_name: "{{ .Values.spec.cluster }}"
```

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
      name: {{ .Values.spec.name }}
    spec:
      chart: kube-prometheus-stack
      repoURL: https://prometheus-community.github.io/helm-charts
      version: "{{ .Values.spec.version }}"
      targetCluster: "{{ .Values.spec.cluster }}"
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
      name: {{ .Values.spec.name | lower | replace "_" "-" }}
    spec:
      chart: {{ .Values.spec.name }}
      repoURL: https://charts.example.com
      version: {{ .Values.spec.version | trimPrefix "v" | quote }}
      targetCluster: {{ .Values.spec.cluster | quote }}
      targetNamespace: {{ printf "app-%s" .Values.spec.name }}
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
