// Copyright © 2025 Jake Rogers <code@supportoss.org>

// Package cmd implements the goDiffIt CLI using Cobra.
// It provides set operations (difference, intersection, union) for comparing files as sets of lines.
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
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
	ignoreFQDN    bool
	pipe          bool
	output        string
	count         bool
	stats         bool
)

// log is the package-level logger instance.
var log = logger.GetLogger()

// config holds runtime configuration for file processing.
// It is populated from CLI flags and passed explicitly to functions.
type config struct {
	caseSensitive bool
	delimiter     string
	extract       *regexp.Regexp
	ignoreFQDN    bool
	pipe          bool
	output        string
	count         bool
	stats         bool
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
		if line != "" {
			set.Add(line)
		}
	}
	if err := reader.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan input: %w", err)
	}
	return set, nil
}

// difference calculates the set difference between fileSetA and fileSetB.
// It populates diffAB with elements in A but not in B.
// If cfg.pipe is false, it also populates diffBA with elements in B but not in A.
func (r *results) difference(cfg *config) {
	r.operation = "difference"
	for _, element := range r.fileSetA.set.Values() {
		if !r.fileSetB.set.Contains(element) {
			r.diffAB.Add(element)
		}
	}
	if cfg.pipe {
		return
	}
	for _, element := range r.fileSetB.set.Values() {
		if !r.fileSetA.set.Contains(element) {
			r.diffBA.Add(element)
		}
	}
}

// union calculates the union of fileSetA and fileSetB.
// It populates diffAB with all elements from both sets.
func (r *results) union() {
	r.operation = "union"
	for _, element := range r.fileSetA.set.Values() {
		r.diffAB.Add(element)
	}
	for _, element := range r.fileSetB.set.Values() {
		r.diffAB.Add(element)
	}
}

// intersection calculates the intersection of fileSetA and fileSetB.
// It populates diffAB with elements present in both sets.
func (r *results) intersection() {
	r.operation = "intersection"
	for _, element := range r.fileSetA.set.Values() {
		if r.fileSetB.set.Contains(element) {
			r.diffAB.Add(element)
		}
	}
}

// symmetricDifference calculates the symmetric difference (XOR) of fileSetA and fileSetB.
// It populates diffAB with elements present in exactly one of the two sets.
func (r *results) symmetricDifference() {
	r.operation = "symmetric-difference"
	for _, element := range r.fileSetA.set.Values() {
		if !r.fileSetB.set.Contains(element) {
			r.diffAB.Add(element)
		}
	}
	for _, element := range r.fileSetB.set.Values() {
		if !r.fileSetA.set.Contains(element) {
			r.diffAB.Add(element)
		}
	}
}

// toSortedSlice converts a hashset to a sorted string slice.
func toSortedSlice(hs *hashset.Set[string]) []string {
	values := hs.Values()
	sort.Strings(values)
	return values
}

// printSet outputs the result sets to stdout or a file.
// When cfg.count is true, it prints only the counts instead of the elements.
// When cfg.pipe is true, it suppresses headers for easier command-line piping.
// For difference operations without pipe mode, it prints both A-B and B-A results.
// If cfg.output is set, results are written to the specified file.
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

		if _, err := fmt.Fprintf(output, "File A: %d unique lines\n", sizeA); err != nil {
			return fmt.Errorf("failed to write stats: %w", err)
		}
		if _, err := fmt.Fprintf(output, "File B: %d unique lines\n", sizeB); err != nil {
			return fmt.Errorf("failed to write stats: %w", err)
		}

		// Calculate overlap (intersection)
		overlap := 0
		for _, element := range r.fileSetA.set.Values() {
			if r.fileSetB.set.Contains(element) {
				overlap++
			}
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
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := &config{
			caseSensitive: caseSensitive,
			delimiter:     delimiter,
			ignoreFQDN:    ignoreFQDN,
			pipe:          pipe,
			output:        output,
			count:         count,
			stats:         stats,
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

		intersection, _ := cmd.Flags().GetBool("intersection")
		union, _ := cmd.Flags().GetBool("union")
		symmetricDiff, _ := cmd.Flags().GetBool("symmetric-difference")

		switch {
		case intersection:
			rs.intersection()
		case union:
			rs.union()
		case symmetricDiff:
			rs.symmetricDifference()
		default:
			rs.difference(cfg)
		}

		log.Debug().Str("operation", rs.operation).Msg("completed")

		return rs.printSet(cfg)
	},
}

// Execute runs the root command and exits with code 1 on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolVarP(&caseSensitive, "case-sensitive", "c", false, "preserve case during comparison (default: case-insensitive)")
	rootCmd.Flags().BoolVar(&count, "count", false, "output only the count of results instead of the elements")
	rootCmd.Flags().StringVarP(&delimiter, "delimiter", "d", ",", "delimiter for splitting lines (default: comma)")
	rootCmd.Flags().StringVarP(&extract, "extract", "e", "", "extract values using regex pattern (use capture group for substring)")
	rootCmd.Flags().BoolVarP(&ignoreFQDN, "ignore-fqdn", "f", false, "strip FQDN suffixes (keep only hostname before first dot)")
	rootCmd.Flags().StringVarP(&output, "output", "o", "", "write output to file instead of stdout")
	rootCmd.Flags().BoolVarP(&pipe, "pipe", "p", false, "suppress headers for piped output")
	rootCmd.Flags().BoolVar(&stats, "stats", false, "show statistics about the file sets (size, overlap, unique elements)")
	rootCmd.Flags().BoolP("intersection", "i", false, "show the intersection of the two files")
	rootCmd.Flags().BoolP("union", "u", false, "show the union of the two files")
	rootCmd.Flags().BoolP("symmetric-difference", "s", false, "show the symmetric difference (XOR) of the two files")
	rootCmd.MarkFlagsMutuallyExclusive("intersection", "union", "symmetric-difference")
	rootCmd.PersistentFlags().CountP("verbose", "v", "increase verbosity (-v=warn, -vv=info, -vvv=debug, -vvvv=trace)")
}
