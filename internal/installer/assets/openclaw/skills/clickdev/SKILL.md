---
name: clickdev
description: Descripción para desarrolladores que quieren retomar en Claude Code un pedido capturado por clickhola; localiza el brief y entrega el siguiente paso.
user-invocable: true
metadata:
  openclaw:
    requires:
      bins:
        - engram
---

# clickdev — puente hacia el pipeline SDD (perfil desarrollador)

## Purpose

Ayudá a un desarrollador a retomar en Claude Code un pedido capturado por clickhola. Aquí solo localiza el brief y entrega el siguiente paso. **NO ejecutes el pipeline** SDD porque OpenClaw **no tiene agentes sdd-***.

## Conversation rules

- **habla en español** con el desarrollador.
- **no traduzcas el brief**; el brief técnico se mantiene en inglés.
- Tu único trabajo es localizar el brief correcto, confirmarlo y dar el siguiente paso.

## Locate the brief

Si el desarrollador te da un nombre de cambio, buscá en Engram el topic:

```text
sdd/{change-name}/elicitation
```

Si no existe, listá briefs recientes con `mem_search` y pedile que elija uno.

## Confirm the brief

Mostrá un resumen corto en español y confirmá que es el brief correcto.

## Next step

Una vez confirmado, decí exactamente:

```text
Abrí Claude Code en el repositorio y ejecutá el flujo SDD para {change-name} (el brief ya está en la memoria compartida).
```

## si no existe el brief

Si `sdd/{change-name}/elicitation` no existe y no hay briefs recientes, explicá que no se encontró un brief. Ofrecé dos opciones:

1. Capturar el requerimiento primero con clickhola.
2. Abrir Claude Code y describir el requerimiento directamente.

- no inicies la entrevista aquí.
- Este skill no conduce elicitation.
