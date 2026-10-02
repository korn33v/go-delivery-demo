# Архитектура в PlantUML

Каждый файл ниже содержит готовый SVG и исходный PlantUML в Markdown. Для семинара достаточно открыть Markdown: устанавливать renderer не нужно. C4-PlantUML include закреплён на v2.12.0. Для повторного рендера изменённого source нужны PlantUML и доступ к указанному include; приложение от renderer не зависит.

1. [C4 Context](01-context.md) — Customer и система целиком.
2. [C4 Container](02-containers.md) — три границы сервисов, ownership и транспорт.
3. [C4 Components](03-components.md) — слои, ports и adapters внутри сервисов.
4. [Sequence](04-sequence.md) — конкретный заказ, SQL и события.

C4 — проектное представление. Динамическая карта находится в Grafana и строится из telemetry работающего кода.
