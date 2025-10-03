# Namespace Filtering met Label Selectors

Deze ECK operator ondersteunt namespace filtering op basis van label selectors, waardoor je meerdere ECK operators kunt draaien die verschillende sets van namespaces beheren.

## Configuratie

### Operator 1 - Beheert alleen namespaces met specifieke labels

```bash
# Operator die alleen namespaces beheert met label "managed-by=eck-operator-1"
./manager \
  --namespace-label-selector="managed-by=eck-operator-1" \
  --operator-namespace=eck-operator-1

# Of met Helm:
helm install eck-operator-1 elastic/eck-operator \
  --namespace eck-operator-1 \
  --create-namespace \
  --set="config.namespaceLabelSelector=managed-by=eck-operator-1"
```

### Operator 2 - Beheert alle andere namespaces (exclusief)

```bash
# Operator die namespaces beheert die NIET gelabeld zijn met "managed-by=eck-operator-1"
./manager \
  --namespace-label-selector="managed-by!=eck-operator-1" \
  --operator-namespace=eck-operator-2

# Of met matchExpressions voor meer complexe selectors:
./manager \
  --namespace-label-selector='{"matchExpressions":[{"key":"managed-by","operator":"NotIn","values":["eck-operator-1"]}]}' \
  --operator-namespace=eck-operator-2
```

## Voorbeelden van Label Selectors

### Eenvoudige equality selectors
```bash
# Alleen namespaces met env=production
--namespace-label-selector="env=production"

# Alleen namespaces met env=production EN team=platform
--namespace-label-selector="env=production,team=platform"
```

### Complex selectors met matchExpressions
```bash
# Namespaces waar env NIET development of staging is
--namespace-label-selector='{"matchExpressions":[{"key":"env","operator":"NotIn","values":["development","staging"]}]}'

# Namespaces die het label "managed-by" hebben (ongeacht de waarde)
--namespace-label-selector='{"matchExpressions":[{"key":"managed-by","operator":"Exists"}]}'

# Namespaces die het label "ignore" NIET hebben
--namespace-label-selector='{"matchExpressions":[{"key":"ignore","operator":"DoesNotExist"}]}'
```

## Namespace Labeling

Label je namespaces om te bepalen welke operator ze beheert:

```yaml
# Voor operator 1
apiVersion: v1
kind: Namespace
metadata:
  name: production-elasticsearch
  labels:
    managed-by: eck-operator-1
    env: production
---
# Voor operator 2 (alle andere)
apiVersion: v1
kind: Namespace
metadata:
  name: development-elasticsearch
  labels:
    env: development
    # Geen managed-by label, dus wordt beheerd door operator 2
```

## Use Cases

### 1. Multi-tenant setup
```bash
# Operator voor tenant A
--namespace-label-selector="tenant=team-a"

# Operator voor tenant B
--namespace-label-selector="tenant=team-b"

# Global operator voor shared resources
--namespace-label-selector="shared=true"
```

### 2. Environment separation
```bash
# Production operator
--namespace-label-selector="env=production"

# Non-production operator
--namespace-label-selector="env!=production"
```

### 3. Regional separation
```bash
# EU operator
--namespace-label-selector="region=eu"

# US operator
--namespace-label-selector="region=us"
```

## Backwards Compatibility

Als je geen `--namespace-label-selector` opgeeft, gedraagt de operator zich zoals voorheen en beheert alle namespaces (behalve als je `--namespaces` gebruikt).

De bestaande `--namespaces` flag blijft werken en wordt gecombineerd met de label selector. Een namespace moet aan beide criteria voldoen om beheerd te worden.