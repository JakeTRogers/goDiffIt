// Copyright © 2025 Jake Rogers <code@supportoss.org>

// Package cmd implements the goDiffIt CLI using Cobra.
// It provides set operations (difference, intersection, union) for comparing files as sets of lines.
package cmd

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/JakeTRogers/goDiffIt/logger"
	"github.com/emirpasic/gods/v2/sets/hashset"
	"github.com/spf13/cobra"
)

// Package-level flag variables bound to Cobra flags.
// These are copied into a config struct at runtime for testability.
var (
	caseSensitive bool
	delimiter     string
	extract       string
	format        string
	ignoreFQDN    bool
	pipe          bool
	output        string
	count         bool
	stats         bool
	trimPrefix    string
	trimSuffix    string
)

// log is the package-level logger instance.
var log = logger.GetLogger()

// config holds runtime configuration for file processing.
// It is populated from CLI flags and passed explicitly to functions.
type config struct {
	caseSensitive bool
	delimiter     string
	extract       *regexp.Regexp
	format        string
	ignoreFQDN    bool
	pipe          bool
	output        string
	count         bool
	stats         bool
	trimPrefix    string
	trimSuffix    string
}

// fileSet associates a file path with its parsed set of normalized lines.
type fileSet struct {
	path string
	set  *hashset.Set[string]
}

// results holds the input file sets, operation name, and computed result sets.
// diffAB contains elements in A but not in B (or union/intersection result).
// diffBA contains elements in B but not in A (only used for difference operation).
type results struct {
	fileSetA  fileSet
	fileSetB  fileSet
	operation string
	diffAB    *hashset.Set[string]
	diffBA    *hashset.Set[string]
}

// Exit codes for meaningful process status.
const (
	exitOK    = 0 // No differences found
	exitDiff  = 1 // Differences found
	exitError = 2 // Error occurred
)

// DiffFoundError indicates differences were found (not a failure).
// This allows distinguishing between errors and successful runs with differences.
type DiffFoundError struct{}

func (DiffFoundError) Error() string { return "differences found" }

// fileToSet reads the file at the given path and returns a set of normalized lines.
// Lines are trimmed, optionally lowercased, split by delimiter, and optionally
// have FQDN suffixes stripped based on the provided config.
// If path is "-", it reads from stdin instead.
func fileToSet(path string, cfg *config) (*hashset.Set[string], error) {
	var reader *bufio.Scanner

	if path == "-" {
		reader = bufio.NewScanner(os.Stdin)
	} else {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return nil, fmt.Errorf("file does not exist: %w", err)
		}

		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open file: %w", err)
		}
		defer func() {
			if cerr := file.Close(); cerr != nil {
				log.Err(cerr).Msg("failed to close file")
			}
		}()
		reader = bufio.NewScanner(file)
	}

	set := hashset.New[string]()
	for reader.Scan() {
		line := strings.TrimSpace(reader.Text())
		if line == "" {
			continue
		}
		if !cfg.caseSensitive {
			line = strings.ToLower(line)
		}
		// Extract using regex if provided (takes precedence over delimiter)
		if cfg.extract != nil {
			matches := cfg.extract.FindStringSubmatch(line)
			if len(matches) > 1 {
				// Use first capture group
				line = strings.TrimSpace(matches[1])
			} else if len(matches) == 1 {
				// No capture group, use full match
				line = strings.TrimSpace(matches[0])
			} else {
				// No match, skip this line
				continue
			}
		} else if strings.Contains(line, cfg.delimiter) {
			line = strings.TrimSpace(strings.Split(line, cfg.delimiter)[0])
		}
		if cfg.ignoreFQDN {
			line = strings.TrimSpace(strings.Split(line, ".")[0])
		}
		if cfg.trimPrefix != "" {
			line = strings.TrimPrefix(line, cfg.trimPrefix)
		}
		if cfg.trimSuffix != "" {
			line = strings.TrimSuffix(line, cfg.trimSuffix)
		}
		if line != "" {
			set.Add(line)
		}
	}
	if err := reader.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan input: %w", err)
	}
	return set, nil
}

// hasDifferences returns true if the result sets contain any elements.
// For difference operations, checks both A-B and B-A sets.
func (r *results) hasDifferences() bool {
	return r.diffAB.Size() > 0 || (r.operation == "difference" && r.diffBA.Size() > 0)
}

// toSortedSlice converts a hashset to a sorted string slice.
func toSortedSlice(hs *hashset.Set[string]) []string {
	return slices.Sorted(slices.Values(hs.Values()))
}

// jsonOutput represents the JSON structure for output.
type jsonOutput struct {
	Operation string              `json:"operation"`
	FileA     string              `json:"fileA"`
	FileB     string              `json:"fileB"`
	Results   map[string][]string `json:"results"`
	Counts    map[string]int      `json:"counts"`
}

// printJSON outputs the results in JSON format.
func (r *results) printJSON(output *os.File) error {
	jo := jsonOutput{
		Operation: r.operation,
		FileA:     r.fileSetA.path,
		FileB:     r.fileSetB.path,
		Results:   make(map[string][]string),
		Counts:    make(map[string]int),
	}

	if r.operation == "difference" {
		jo.Results["A-B"] = toSortedSlice(r.diffAB)
		jo.Results["B-A"] = toSortedSlice(r.diffBA)
		jo.Counts["A-B"] = r.diffAB.Size()
		jo.Counts["B-A"] = r.diffBA.Size()
	} else {
		jo.Results[r.operation] = toSortedSlice(r.diffAB)
		jo.Counts[r.operation] = r.diffAB.Size()
	}

	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(jo); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	return nil
}

// printCSV outputs the results in CSV format.
// For difference operations, includes a "set" column indicating A-B or B-A.
func (r *results) printCSV(output *os.File) error {
	writer := csv.NewWriter(output)
	defer writer.Flush()

	if r.operation == "difference" {
		// Write header
		if err := writer.Write([]string{"set", "value"}); err != nil {
			return fmt.Errorf("failed to write CSV header: %w", err)
		}
		// Write A-B values
		for _, v := range toSortedSlice(r.diffAB) {
			if err := writer.Write([]string{"A-B", v}); err != nil {
				return fmt.Errorf("failed to write CSV row: %w", err)
			}
		}
		// Write B-A values
		for _, v := range toSortedSlice(r.diffBA) {
			if err := writer.Write([]string{"B-A", v}); err != nil {
				return fmt.Errorf("failed to write CSV row: %w", err)
			}
		}
	} else {
		// Write header
		if err := writer.Write([]string{"value"}); err != nil {
			return fmt.Errorf("failed to write CSV header: %w", err)
		}
		// Write values
		for _, v := range toSortedSlice(r.diffAB) {
			if err := writer.Write([]string{v}); err != nil {
				return fmt.Errorf("failed to write CSV row: %w", err)
			}
		}
	}

	return nil
}

// printSet outputs the result sets to stdout or a file.
// When cfg.count is true, it prints only the counts instead of the elements.
// When cfg.pipe is true, it suppresses headers for easier command-line piping.
// For difference operations without pipe mode, it prints both A-B and B-A results.
// If cfg.output is set, results are written to the specified file.
// cfg.format controls output format: "text" (default), "json", or "csv".
func (r *results) printSet(cfg *config) error {
	var output *os.File
	var err error

	if cfg.output != "" {
		output, err = os.Create(cfg.output)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer func() {
			if cerr := output.Close(); cerr != nil {
				log.Err(cerr).Msg("failed to close output file")
			}
		}()
	} else {
		output = os.Stdout
	}

	// JSON format output
	if cfg.format == "json" {
		return r.printJSON(output)
	}

	// CSV format output
	if cfg.format == "csv" {
		return r.printCSV(output)
	}

	// Count mode: only output counts
	if cfg.count {
		if r.operation == "difference" {
			if _, err := fmt.Fprintf(output, "A-B: %d\n", r.diffAB.Size()); err != nil {
				return fmt.Errorf("failed to write count: %w", err)
			}
			if _, err := fmt.Fprintf(output, "B-A: %d\n", r.diffBA.Size()); err != nil {
				return fmt.Errorf("failed to write count: %w", err)
			}
		} else {
			if _, err := fmt.Fprintf(output, "%d\n", r.diffAB.Size()); err != nil {
				return fmt.Errorf("failed to write count: %w", err)
			}
		}
		return nil
	}

	// Stats mode: output detailed statistics
	if cfg.stats {
		sizeA := r.fileSetA.set.Size()
		sizeB := r.fileSetB.set.Size()
		overlap := r.fileSetA.set.Intersection(r.fileSetB.set).Size()

		if _, err := fmt.Fprintf(output, "File A: %d unique lines\n", sizeA); err != nil {
			return fmt.Errorf("failed to write stats: %w", err)
		}
		if _, err := fmt.Fprintf(output, "File B: %d unique lines\n", sizeB); err != nil {
			return fmt.Errorf("failed to write stats: %w", err)
		}

		if sizeA > 0 && sizeB > 0 {
			pctA := float64(overlap) / float64(sizeA) * 100
			pctB := float64(overlap) / float64(sizeB) * 100
			if _, err := fmt.Fprintf(output, "Overlap: %d (%.1f%% of A, %.1f%% of B)\n", overlap, pctA, pctB); err != nil {
				return fmt.Errorf("failed to write stats: %w", err)
			}
		} else {
			if _, err := fmt.Fprintf(output, "Overlap: %d\n", overlap); err != nil {
				return fmt.Errorf("failed to write stats: %w", err)
			}
		}

		onlyA := sizeA - overlap
		onlyB := sizeB - overlap
		if _, err := fmt.Fprintf(output, "Only in A: %d\n", onlyA); err != nil {
			return fmt.Errorf("failed to write stats: %w", err)
		}
		if _, err := fmt.Fprintf(output, "Only in B: %d\n", onlyB); err != nil {
			return fmt.Errorf("failed to write stats: %w", err)
		}

		return nil
	}

	if !cfg.pipe {
		var header string
		switch r.operation {
		case "intersection":
			header = fmt.Sprintf("Intersection of %s and %s:\n", r.fileSetA.path, r.fileSetB.path)
		case "union":
			header = fmt.Sprintf("Union of %s and %s:\n", r.fileSetA.path, r.fileSetB.path)
		case "difference":
			header = fmt.Sprintf("Difference of %s - %s:\n", r.fileSetA.path, r.fileSetB.path)
		case "symmetric-difference":
			header = fmt.Sprintf("Symmetric difference of %s and %s:\n", r.fileSetA.path, r.fileSetB.path)
		default:
			return fmt.Errorf("invalid operation: %s", r.operation)
		}
		if _, err := fmt.Fprint(output, header); err != nil {
			return fmt.Errorf("failed to write header: %w", err)
		}
	}

	for _, element := range toSortedSlice(r.diffAB) {
		if _, err := fmt.Fprintln(output, element); err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}
	}

	if r.operation == "difference" && !cfg.pipe {
		header := fmt.Sprintf("\nDifference of %s - %s:\n", r.fileSetB.path, r.fileSetA.path)
		if _, err := fmt.Fprint(output, header); err != nil {
			return fmt.Errorf("failed to write header: %w", err)
		}
		for _, element := range toSortedSlice(r.diffBA) {
			if _, err := fmt.Fprintln(output, element); err != nil {
				return fmt.Errorf("failed to write output: %w", err)
			}
		}
	}
	return nil
}

var rootCmd = &cobra.Command{
	Use:     "goDiffIt [fileA] [fileB]",
	Version: "v1.0.6",
	Short:   "goDiffIt is a CLI tool for comparing files/lists.",
	Long: `goDiffIt is a CLI tool for comparing files/lists and explaining their differences. It can perform set operations such as
union, intersection, and difference. This is very helpful for comparing data from different sources, and spotting gaps.

It is case insensitive by default, but can be configured to be case sensitive with the --case-sensitive flag. It can
also be configured to ignore fully qualified domain names (FQDNs). This is useful when one dataset is fully qualified
and another is not.

It can also be used to compare first column CSV files, or a CSV file and a text file. The delimiter for CSV files is
comma by default, but any character can be specified via the --delimiter flag.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("requires at least two args: fileA and fileB")
		}
		return nil
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		verboseCount, _ := cmd.Flags().GetCount("verbose")
		logger.SetLogLevel(verboseCount)
	},
	// SilenceErrors prevents Cobra from printing DiffFoundError
	SilenceErrors: true,
	// SilenceUsage prevents usage output on DiffFoundError
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := &config{
			caseSensitive: caseSensitive,
			delimiter:     delimiter,
			format:        format,
			ignoreFQDN:    ignoreFQDN,
			pipe:          pipe,
			output:        output,
			count:         count,
			stats:         stats,
			trimPrefix:    trimPrefix,
			trimSuffix:    trimSuffix,
		}

		// Compile extract regex if provided
		if extract != "" {
			re, err := regexp.Compile(extract)
			if err != nil {
				return fmt.Errorf("invalid extract regex: %w", err)
			}
			cfg.extract = re
		}

		// Log flag values at debug level
		log.Debug().
			Bool("case-sensitive", cfg.caseSensitive).
			Str("delimiter", cfg.delimiter).
			Bool("ignore-fqdn", cfg.ignoreFQDN).
			Bool("pipe", cfg.pipe).
			Msg("flags")

		setA, err := fileToSet(args[0], cfg)
		if err != nil {
			return fmt.Errorf("file A: %w", err)
		}
		setB, err := fileToSet(args[1], cfg)
		if err != nil {
			return fmt.Errorf("file B: %w", err)
		}

		rs := results{
			fileSetA: fileSet{path: args[0], set: setA},
			fileSetB: fileSet{path: args[1], set: setB},
			diffAB:   hashset.New[string](),
			diffBA:   hashset.New[string](),
		}

		log.Debug().Str("fileA", rs.fileSetA.path).Str("fileB", rs.fileSetB.path).Msg("processing")

		intersectionFlag, _ := cmd.Flags().GetBool("intersection")
		unionFlag, _ := cmd.Flags().GetBool("union")
		symmetricDiffFlag, _ := cmd.Flags().GetBool("symmetric-difference")

		switch {
		case intersectionFlag:
			rs.operation = "intersection"
			rs.diffAB = rs.fileSetA.set.Intersection(rs.fileSetB.set)
		case unionFlag:
			rs.operation = "union"
			rs.diffAB = rs.fileSetA.set.Union(rs.fileSetB.set)
		case symmetricDiffFlag:
			rs.operation = "symmetric-difference"
			rs.diffAB = rs.fileSetA.set.Difference(rs.fileSetB.set).Union(rs.fileSetB.set.Difference(rs.fileSetA.set))
		default:
			rs.operation = "difference"
			rs.diffAB = rs.fileSetA.set.Difference(rs.fileSetB.set)
			if !cfg.pipe {
				rs.diffBA = rs.fileSetB.set.Difference(rs.fileSetA.set)
			}
		}

		log.Debug().Str("operation", rs.operation).Msg("completed")

		if err := rs.printSet(cfg); err != nil {
			return err
		}

		// Return DiffFoundError if there are differences
		if rs.hasDifferences() {
			return DiffFoundError{}
		}
		return nil
	},
}

// Execute runs the root command and exits with appropriate code.
// Exit codes: 0 = no differences, 1 = differences found, 2 = error.
func Execute() {
	err := rootCmd.Execute()
	if err == nil {
		os.Exit(exitOK)
	}
	if _, ok := err.(DiffFoundError); ok {
		os.Exit(exitDiff)
	}
	// Print actual errors (SilenceErrors is true, so we do it manually)
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(exitError)
}

func init() {
	rootCmd.Flags().BoolVarP(&caseSensitive, "case-sensitive", "c", false, "preserve case during comparison (default: case-insensitive)")
	rootCmd.Flags().BoolVar(&count, "count", false, "output only the count of results instead of the elements")
	rootCmd.Flags().StringVarP(&delimiter, "delimiter", "d", ",", "delimiter for splitting lines (default: comma)")
	rootCmd.Flags().StringVarP(&extract, "extract", "e", "", "extract values using regex pattern (use capture group for substring)")
	rootCmd.Flags().StringVar(&format, "format", "text", "output format: text, json, or csv")
	rootCmd.Flags().BoolVarP(&ignoreFQDN, "ignore-fqdn", "f", false, "strip FQDN suffixes (keep only hostname before first dot)")
	rootCmd.Flags().StringVarP(&output, "output", "o", "", "write output to file instead of stdout")
	rootCmd.Flags().BoolVarP(&pipe, "pipe", "p", false, "suppress headers for piped output")
	rootCmd.Flags().BoolVar(&stats, "stats", false, "show statistics about the file sets (size, overlap, unique elements)")
	rootCmd.Flags().StringVar(&trimPrefix, "trim-prefix", "", "remove specified prefix from each line")
	rootCmd.Flags().StringVar(&trimSuffix, "trim-suffix", "", "remove specified suffix from each line")
	rootCmd.Flags().BoolP("intersection", "i", false, "show the intersection of the two files")
	rootCmd.Flags().BoolP("union", "u", false, "show the union of the two files")
	rootCmd.Flags().BoolP("symmetric-difference", "s", false, "show the symmetric difference (XOR) of the two files")
	rootCmd.MarkFlagsMutuallyExclusive("intersection", "union", "symmetric-difference")
	rootCmd.MarkFlagsMutuallyExclusive("format", "count")
	rootCmd.MarkFlagsMutuallyExclusive("format", "stats")
	rootCmd.PersistentFlags().CountP("verbose", "v", "increase verbosity (-v=warn, -vv=info, -vvv=debug, -vvvv=trace)")
}
