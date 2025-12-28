# Copilot Instructions for goDiffIt

## Project Overview

goDiffIt is a Go 1.25 CLI tool that compares two files using set operations (difference, intersection, union). Unlike `diff`, it treats files as sets of lines—order and duplicates don't matter.

## Architecture

```
main.go          → Entry point, calls cmd.Execute()
cmd/root.go      → All CLI logic: Cobra command, flags, set operations
logger/logger.go → Zerolog wrapper with verbosity levels (-v to -vvvv)
```

**Key types in `cmd/root.go`:**
- `fileSet` – wraps a file path and its parsed `hashset.Set`
- `results` – holds both file sets, operation name, and result sets (setAB, setBA)

**Data flow:** File → `fileToSet()` (normalize lines) → set operation → `printSet()` output

## Code Patterns

**Error handling:** Wrap errors with context using `fmt.Errorf("description: %w", err)`

**Logging:** Use the package-level logger `l` from `logger.GetLogger()`:
```go
l.Debug().Str("key", value).Send()
l.Fatal().Err(err).Send()  // For unrecoverable errors
```

**Adding CLI flags:** Define in `init()`, use `rootCmd.Flags()` for command-specific or `PersistentFlags()` for global. Mutually exclusive flags use `MarkFlagsMutuallyExclusive()`.

## Development Commands

```bash
# Run tests
go test ./... -v

# Coverage report (cmd package must maintain ≥85%)
go test ./... -cover

# Build
go build -o godiffit .
```

## Testing Patterns

Tests live in `cmd/root_test.go`. Follow these patterns:

- **Temp files:** Use `writeTempFile(t, lines)` helper to create test input
- **Flag isolation:** Use `withFlags(t, caseSensitive, delimiter, ignoreFQDN)` to set flags with automatic cleanup
- **Output capture:** Use `captureOutput(t, fn)` to capture stdout
- **CLI cleanup:** Use `withCLICleanup(t)` when testing full command execution to reset `os.Args` and flags

Example test structure:
```go
func TestFeature(t *testing.T) {
    withFlags(t, false, ",", true)
    path := writeTempFile(t, []string{"line1", "line2"})
    // ... test logic
}
```

## Commit Convention

Uses [Conventional Commits](https://www.conventionalcommits.org/)

## Pre-commit Hooks

Hooks run automatically on commit. Install with:
```bash
pre-commit install --hook-type pre-commit --hook-type commit-msg
```

Checks: go test, coverage ≥85%, golangci-lint, commitizen message format.
