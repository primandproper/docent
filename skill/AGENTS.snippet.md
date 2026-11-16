## Code tours

This repository's guided tours live in `.tours/` and are walked with [Docent](https://github.com/primandproper/docent).

- To write or update a tour, run `docent guide` and follow it exactly.
- Check every anchor with `docent resolve <tour> --step <id>`.
- A tour isn't done until `docent validate --strict --json <tour>` prints no diagnostics.
- `docent stale --json` lists tours the code has moved away from, with the diffs to work from.
