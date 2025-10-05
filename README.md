
# goDiffIt

## Description

goDiffIt is a command-line utility written in Go that uses boolean operations(difference, intersection, and union) to compare 2 files. It can be used to compare text files, CSV files, or any other file that can be read as a series of lines. It can also read from file descriptors, so you can use it to compare the output of other commands.

It is different from the diff command in that it does not compare the contents of the files line-by-line. Instead, it compares the files as sets of lines. This means that it can be used to compare files that have the same lines in different orders, or files that have the same lines repeated multiple times.

A common use case for this tool is comparing lists of servers from different sources to see which servers are missing from one or the other. Alternatively, after looping over a few thousand systems to return configuration data in CSV format, this tool can identify any systems that didn't return results.

## Installation

1. Download the binary for your preferred platform from the [releases](https://github.com/JakeTRogers/goDiffIt/releases) page
2. Extract the archive. It contains this readme, a copy of the Apache 2.0 license, and the goDiffIt binary.
3. Copy the binary to a directory in your `$PATH`. i.e. `/usr/local/bin`

## Usage

To use goDiffIt, you need to pass two arguments: the paths to the two files you want to compare:

```shell
./godiffit fileA.txt fileB.txt
```

This will print lines that appear in fileA.txt but not in fileB.txt, and lines that appear in fileB.txt but not in fileA.txt.

If you're comparing CSV files, you can specify the delimiter with the --delimiter flag:

```bash
./godiffit --delimiter=";" fileA.csv fileB.csv
```

The tool can also read from file descriptors:

```bash
./godiffit <(cut -d, -f2,3 fileA) <(grep -v '^#' fileB)
```

## Examples

If `fileA.txt` contains:

```text
apple
banana
cherry
orange
```

and `fileB.txt` contains:

```text
BANANA.example.com,abc,xyz
Cherry,stuff, and, things
date
```

`goDiffIt fileA.txt fileB.txt --ignore-fqdn` will output:

```text
Difference of fileA.txt - fileB.txt:
apple
orange

Difference of fileB.txt - fileA.txt:
date
```

`goDiffIt fileA.txt fileB.txt --intersection --ignore-fqdn` will output:

```text
Intersection of fileA.txt and fileB.txt:
banana
cherry
```

`goDiffIt fileA.txt fileB.txt --union --ignore-fqdn` will output:

```text
Union of fileA.txt and fileB.txt:
apple
banana
cherry
date
orange
```

## Development

### Running Tests

```bash
go test ./... -v
```

### Coverage Report

```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Pre-commit Hooks

This project uses [pre-commit](https://pre-commit.com/) hooks to ensure code quality. The hooks run:

- **go test**: Runs all tests with race detection
- **go test coverage**: Ensures cmd package maintains ≥85% coverage
- **golangci-lint**: Comprehensive Go linting
- **commitizen**: Enforces conventional commit messages

To install pre-commit hooks:

```bash
# Install pre-commit (if not already installed)
pip install pre-commit

# Install the git hooks
pre-commit install --hook-type pre-commit --hook-type commit-msg

# Run hooks manually on all files
pre-commit run --all-files
```

The hooks will automatically run before each commit. To skip hooks temporarily:

```bash
git commit --no-verify
```
