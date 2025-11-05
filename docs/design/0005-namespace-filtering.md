# Namespace Filtering met Label Selectors

Deze ECK operator ondersteunt namespace filtering op basis van label selectors, waardoor je meerdere ECK operators kunt draaien die verschillende sets van namespaces beheren.

## Inleiding

Namespace filtering is een krachtige functie die je in staat stelt om de reikwijdte van je ECK operator te beperken tot specifieke namespaces binnen je Kubernetes cluster. Dit wordt bereikt door het toepassen van label selectors, die fungeren als een filtermechanisme om alleen die namespaces te selecteren die voldoen aan bepaalde criteria.

## Hoe het Werkt

De ECK operator maakt gebruik van Kubernetes label selectors om namespaces te filteren. Een label selector is een uitdrukking die bepaalt welke labels overeenkomen met de gespecificeerde criteria. Door deze selectors te gebruiken, kun je de ECK operator configureren om alleen die namespaces te beheren die de juiste labels hebben.

Bijvoorbeeld, als je een label selector hebt gedefinieerd als `environment: production`, dan zal de ECK operator alleen van toepassing zijn op die namespaces die het label `environment` hebben met de waarde `production`.

## Voordelen van Namespace Filtering

- **Gerichte Toepassingen**: Je kunt de ECK operator richten op specifieke namespaces, wat handig is in omgevingen waar meerdere teams of projecten dezelfde Kubernetes cluster delen.
- **Resource Optimalisatie**: Door de reikwijdte van de operator te beperken, worden de resources efficiënter gebruikt en wordt de kans op conflicten tussen verschillende toepassingen verminderd.
- **Veiligheid en Isolatie**: Namespace filtering helpt bij het handhaven van veiligheid en isolatie tussen verschillende delen van je applicatie of tussen verschillende applicaties die op dezelfde cluster draaien.

## Configuratie Voorbeeld

Hier is een voorbeeld van hoe je namespace filtering kunt configureren met behulp van label selectors in je ECK operator manifest:

```yaml
apiVersion: operator.elastic.co/v1
kind: Elasticsearch
metadata:
  name: mijn-elk-cluster
  namespace: mijn-namespace
  labels:
    environment: production
spec:
  ...
```

In dit voorbeeld zal de ECK operator alleen van toepassing zijn op de namespace `mijn-namespace` als deze het label `environment: production` heeft.

## Conclusie

Namespace filtering met label selectors biedt een flexibele en krachtige manier om de reikwijdte van je ECK operator te beheren. Door gebruik te maken van deze functie, kun je efficiënter werken met meerdere namespaces binnen je Kubernetes cluster en tegelijkertijd een hoge mate van isolatie en veiligheid handhaven.