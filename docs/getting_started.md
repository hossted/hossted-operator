# Installation

- Install hossted operator using helm charts.

```
helm upgrade --install operator . -n operator --set env.HOSSTED_API_URL="<>",env.HOSSTED_AUTH_TOKEN="<>",env.EMAIL_ID="<>" 
```
