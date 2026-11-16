A purge job retries until it succeeds or runs out of attempts.

```docent-state
states: [queued, { id: running, label: Running }, done, failed]
transitions:
  - { from: queued, to: running, on: start }
  - { from: running, to: running, on: retry }
  - { from: running, to: done, on: succeed }
  - { from: running, to: failed, on: give up }
run: [start, retry, retry, succeed]
```
