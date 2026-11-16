Reads are the hot path, so `GetItem` does as little as it can.

| Outcome | What happens |
| --- | --- |
| Cache hit | The edge answers; this code never runs |
| Cache miss | The store is read and the response is cached |

A **404** is cached too, briefly, so a missing item can't be used to hammer the store.
