A write reaches the cache in four messages.

```docent-sequence
title: Invalidating one item
participants: [client, api, queue, worker, cache]
messages:
  - { from: client, to: api, label: "PUT /items/42" }
  - { from: api, to: queue, label: "enqueue invalidate(42)", async: true, step: flow }
  - { from: queue, to: worker, label: "deliver" }
  - { from: worker, to: cache, label: "PURGE /items/42", step: timeline }
```
