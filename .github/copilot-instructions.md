# Copilot Instructions for goDiffIt

## Project Overview

goDiffIt is a Go 1.25 CLI tool that compares two files using set operations (difference, intersection, union, symmetric-difference). Unlike `diff`, it treats files as sets of lines—order and duplicates don't matter.

## Architecture

```text
main.go          → Entry point, calls cmd.Execute()
cmd/root.go      → All CLI logic: Cobra command, flags, set operations
logger/logger.go → Zerolog wrapper with verbosity levels (-v to -vvvv)
```

**Key types in `cmd/root.go`:**

- `config` – runtime configuration populated from CLI flags, passed explicitly to functions
- `fileSet` – wraps a file path and its parsed `hashset.Set`
- `results` – holds both file sets, operation name, and computed result sets (diffAB, diffBA)

**Data flow:** File → `fileToSet()` (normalize lines) → set operation → `printSet()` output

**Exit codes:** `0` = no differences, `1` = differences found (`DiffFoundError`), `2` = error occurred

## Code Patterns

**Error handling:** Wrap errors with context using `fmt.Errorf("description: %w", err)`

**Logging:** Use the package-level logger `log` from `logger.GetLogger()`:

```go
log.Debug().Str("key", value).Msg("description")
log.Err(err).Msg("failed to close file")  // For recoverable errors
```

**Adding CLI flags:** Define in `init()`, use `rootCmd.Flags()` for command-specific or `PersistentFlags()` for global. Mutually exclusive flags use `MarkFlagsMutuallyExclusive()`.

**Output formats:** Support text (default), JSON, and CSV via `--format` flag. Use `printJSON()`, `printCSV()`, or text output in `printSet()`.

## Development Commands

```bash
go test ./... -v           # Run tests
go test ./... -cover       # Coverage (must maintain ≥80%)
go build -o godiffit .     # Build binary
```

## Testing Patterns

Tests live in `cmd/root_test.go`. Use these helpers:

- `writeTempFile(t, lines)` – creates temp file with test data
- `testConfig(caseSens, delim, ignoreFQDN, pipeMode)` – creates config struct for unit tests
- `captureOutput(t, fn)` – captures stdout during function execution
- `withCLICleanup(t)` – resets `os.Args` and Cobra flags for CLI integration tests
- `makeSet(values...)` – creates hashset from string values
- `makeResultsWithOp(setA, setB, op, pipeMode)` – creates results struct with operation applied

Example test structure:

```go
func TestFeature(t *testing.T) {
    t.Parallel()
    cfg := testConfig(false, ",", true, false)
    path := writeTempFile(t, []string{"line1", "line2"})
    set, err := fileToSet(path, cfg)
    // assertions...
}
```

## Commit Convention

Uses [Conventional Commits](https://www.conventionalcommits.org/) (enforced by pre-commit hooks).
