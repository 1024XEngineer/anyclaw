---
name: weather
description: Get current weather and forecasts with no API key required.
homepage: https://wttr.in/:help
metadata: {"clawdbot":{"emoji":"weather","requires":{"bins":["curl"]}}}
---

# Weather

Use this skill for current conditions and short forecasts. It does not require
an API key.

## wttr.in

Quick current weather:

```bash
curl -s "wttr.in/London?format=3"
```

Compact format:

```bash
curl -s "wttr.in/London?format=%l:+%c+%t+%h+%w"
```

Full forecast:

```bash
curl -s "wttr.in/London?T"
```

Tips:

- URL-encode spaces: `wttr.in/New+York`
- Airport codes work: `wttr.in/JFK`
- Metric units: `?m`
- US units: `?u`
- Today only: `?1`
- Current only: `?0`

## Open-Meteo

Use Open-Meteo when a JSON response is better for automation:

```bash
curl -s "https://api.open-meteo.com/v1/forecast?latitude=51.5&longitude=-0.12&current_weather=true"
```

Docs: https://open-meteo.com/en/docs
