The walker is the one place a step meets the code. The app's service and `docent validate` both call it, so there is
no second implementation to drift.

For a file step it answers three questions in order: which repo, at which ref, and where in the file.

```docent-sequence
title: Resolving one step
participants: [caller, walker, repos, git, anchor]
messages:
  - { from: caller, to: walker, label: "Step(tour, index)" }
  - { from: walker, to: repos, label: "where is this repo?" }
  - { from: walker, to: git, label: "what does the ref point at?" }
  - { from: walker, to: anchor, label: "resolve, following drift", step: locate }
  - { from: anchor, to: git, label: "read the file", step: confine }
  - { from: walker, to: caller, label: "range + status" }
```
