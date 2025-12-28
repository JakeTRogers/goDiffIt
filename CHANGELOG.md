## v2.0.0 (2025-12-28)

### Feat

- implement exit codes and DiffFoundError for better process status handling
- add JSON and CSV output formats for result sets with corresponding tests
- add trim prefix and suffix options for line processing and corresponding tests
- add regex extraction option for file parsing and corresponding tests
- enhance fileToSet to support reading from stdin and add corresponding tests
- add stats mode to display detailed statistics about file sets
- add count mode to output only the counts of results
- add output option to print results to a file or stdout
- add symmetric difference operation and corresponding tests

### Fix

- **deps**: bump github.com/spf13/cobra from 1.10.1 to 1.10.2

### Refactor

- replace sort with slices for improved performance; use hashset boolean diff functions
- large refactor of goDiffIt CLI and update dependencies to follow go.instructions.md

## v1.0.6 (2025-10-05)

### Fix

- handle scanner errors in fileToSet function
- trim whitespace from lines in fileToSet function
- ensure file close is deferred in fileToSet function

## v1.0.5 (2025-10-05)

### Fix

- **deps**: bump github.com/spf13/cobra from 1.9.1 to 1.10.1
- **deps**: bump github.com/spf13/pflag from 1.0.7 to 1.0.10
- bump Go version from 1.23 to 1.25

## v1.0.4 (2025-08-17)

### Fix

- handle file close error in fileToSet function
- **deps**: bump github.com/spf13/pflag from 1.0.6 to 1.0.7

## v1.0.3 (2025-06-01)

### Fix

- update Go version from 1.21 to 1.23
- **devcontainer**: bump Go image version from 1.21 to 1.23
- **deps**: bump github.com/rs/zerolog from 1.33.0 to 1.34.0
- **deps**: bump github.com/spf13/cobra from 1.8.0 to 1.9.1
- **deps**: bump github.com/spf13/pflag from 1.0.5 to 1.0.6

## v1.0.2 (2024-05-27)

### Fix

- **deps**: bump github.com/rs/zerolog from 1.32.0 to 1.33.0

## v1.0.1 (2024-02-05)

### Fix

- **deps**: bump github.com/rs/zerolog from 1.31.0 to 1.32.0

## v1.0.0 (2024-01-16)
