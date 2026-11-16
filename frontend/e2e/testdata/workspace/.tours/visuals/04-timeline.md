Retries back off exponentially.

```docent-timeline
title: Retries with backoff
unit: ms
lanes: [{ id: worker, label: Worker }, { id: cache, label: Edge cache }]
events:
  - { lane: worker, at: 0, label: "attempt 1" }
  - { lane: worker, at: 200, label: "attempt 2" }
  - { lane: worker, at: 600, label: "attempt 3", step: state }
  - { lane: cache, at: 640, label: "purged" }
```

```docent-links
- { kind: grafana, title: Purge latency, url: "{{grafana}}/d/purge?var-env={{env}}" }
- { kind: logs, title: Retry logs, url: "{{logs}}/search?q=retry" }
```

```docent-flow
nodes: [a, b
```
