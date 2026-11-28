# AGENTS.md

goDiffIt compares two files as **sets of lines**: line order and duplicates never matter. Every operation (difference, intersection, union, symmetric-difference) and every test follows from that.

## Conventions

- **Exit codes are part of the contract** (ADR 0001): `0` the Result is empty, `1` it holds at least one Element, `2` error, whatever the Operation. So `--union` of non-empty inputs exits 1 even when they are identical, and an `--intersection` of disjoint inputs exits 0. Exit 1 comes from `DiffFoundError`, a result rather than a failure, despite its name; treat it as success in tests and callers.
- **Every output change covers all three `--format` modes**: text, `json` (`printJSON`) and `csv` (`printCSV`), dispatched from `printSet`.
- **Log through the package-level `log`** (`logger.GetLogger()`, zerolog): `log.Debug()` for tracing, `log.Err(err)` for errors handled without aborting, such as a failed close.
- **Tests run in parallel** (`t.Parallel()`). Build on the helpers at the top of `cmd/root_test.go`: CLI tests go through `runCLIWithArgs` / `runCLIExpectError`, which build a fresh `newRootCmd()` per call; leave global Cobra state and `os.Args` untouched.
- **Keep coverage at or above 80%.** CI prints coverage without gating on it, so check `go test ./... -cover` yourself.

## Agent skills

### Issue tracker

GitHub Issues on JakeTRogers/goDiffIt, through `gh`. See `docs/agents/issue-tracker.md`.

### Triage labels

The five default labels, each named after its role. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.
