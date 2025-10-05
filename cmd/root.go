/*
Copyright © 2024 Jake Rogers <code@supportoss.org>
*/
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/JakeTRogers/goDiffIt/logger"
	"github.com/alexandrestein/gods/sets/hashset"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	caseSensitive bool
	delimiter     string
	ignoreFQDN    bool
	pipe          bool
	l             = logger.GetLogger()
)

type fileSet struct {
	path string
	set  hashset.Set
}

type results struct {
	fileSetA  fileSet
	fileSetB  fileSet
	operation string
	setAB     hashset.Set
	setBA     hashset.Set
}

/*
fileToSet reads the file specified by fs.path and adds each non-empty line to the set.
If caseSensitive is false, it converts each line to lowercase before adding it to the set.
If ignoreFQDN is true, it splits each line by dot and adds the first element to the set.
Returns an error if the file does not exist or if there is an error while reading the file.
*/
func (fs *fileSet) fileToSet() error {
	// ensure the file exists
	if _, err := os.Stat(fs.path); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %w", err)
	}

	// read the file
	file, err := os.Open(fs.path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	// add each line to the set
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// if line is empty or contains only whitespace, skip it
		if len(strings.TrimSpace(line)) == 0 {
			continue
		}
		// convert the line to lowercase if caseSensitive is false
		if !caseSensitive {
			line = strings.ToLower(line)
		}
		// split the line by delimiter and take the first element
		if strings.Contains(line, delimiter) {
			line = strings.Split(line, delimiter)[0]
		}
		// split the line by dot and take the first element if ignoreFQDN is set
		if ignoreFQDN {
			line = strings.Split(line, ".")[0]
		}
		fs.set.Add(line)
	}
	return nil
}

/*
difference calculates the difference between two sets and stores the result in the results struct.  It iterates over
each element in fileSetA and checks if it exists in fileSetB. If an element is not found in fileSetB, it is added to the
resultAB set. If the 'pipe' flag is not set, it also iterates over each element in fileSetB and checks if it exists in
fileSetA. If an element is not found in fileSetA, it is added to the resultBA set.
*/
func (r *results) difference() {
	r.operation = "difference"
	for _, element := range r.fileSetA.set.Values() {
		if !r.fileSetB.set.Contains(element) {
			r.setAB.Add(element)
		}
	}
	if !pipe {
		for _, element := range r.fileSetB.set.Values() {
			if !r.fileSetA.set.Contains(element) {
				r.setBA.Add(element)
			}
		}
	}
}

// union calculates the union of two sets and stores the result in the results struct.
func (r *results) union() {
	r.operation = "union"
	for _, element := range r.fileSetA.set.Values() {
		r.setAB.Add(element)
	}
	for _, element := range r.fileSetB.set.Values() {
		r.setAB.Add(element)
	}
}

// intersection calculates the intersection of two sets and stores the result in the results struct.
func (r *results) intersection() {
	r.operation = "intersection"
	for _, element := range r.fileSetA.set.Values() {
		if r.fileSetB.set.Contains(element) {
			r.setAB.Add(element)
		}
	}
}

// convertToSortedStringSlice converts a hashset.Set to a sorted string slice.
func convertToSortedStringSlice(hs hashset.Set) []string {
	s := make([]string, hs.Size())
	for i, v := range hs.Values() {
		s[i] = v.(string)
	}
	sort.Strings(s)
	return s
}

/*
printSet prints the result sets based on the operation performed.  The function handles printing the second set when the
operation is "difference", showing but A - B and B - A.  If the pipe flag is true, and the operation is "difference", it
only prints the first set to allow command line piping.
It returns an error if the operation is invalid.
*/
func (r *results) printSet() error {
	if !pipe {
		switch r.operation {
		case "intersection":
			fmt.Printf("Intersection of %s and %s:\n", r.fileSetA.path, r.fileSetB.path)
		case "union":
			fmt.Printf("Union of %s and %s:\n", r.fileSetA.path, r.fileSetB.path)
		case "difference":
			fmt.Printf("Difference of %s - %s:\n", r.fileSetA.path, r.fileSetB.path)
		default:
			return fmt.Errorf("invalid operation: %s", r.operation)
		}
	}
	for _, element := range convertToSortedStringSlice(r.setAB) {
		fmt.Println(element)
	}
	// for difference, print the second set showing B - A if the pipe flag is not set
	if r.operation == "difference" && !pipe {
		fmt.Printf("\nDifference of %s - %s:\n", r.fileSetB.path, r.fileSetA.path)
		for _, element := range convertToSortedStringSlice(r.setBA) {
			fmt.Println(element)
		}
	}
	return nil
}

var rootCmd = &cobra.Command{
	Use:     "goDiffIt [fileA] [fileB]",
	Version: "v1.0.5",
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
	Run: func(cmd *cobra.Command, args []string) {
		// loop through flags and print their values
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			l.Debug().Str("flag", f.Name).Str("value", f.Value.String()).Send()
		})

		fsA := fileSet{path: args[0], set: *hashset.New()}
		if err := fsA.fileToSet(); err != nil {
			l.Fatal().Err(err).Send()
		}
		fsB := fileSet{path: args[1], set: *hashset.New()}
		if err := fsB.fileToSet(); err != nil {
			l.Fatal().Err(err).Send()
		}

		rs := results{
			fileSetA: fsA,
			fileSetB: fsB,
			setAB:    *hashset.New(),
			setBA:    *hashset.New(),
		}
		l.Debug().Str("rs.fileSetA.path", fsA.path).Send()
		l.Debug().Str("rs.fileSetB.path", fsB.path).Send()
		if cmd.Flags().Changed("intersection") {
			rs.intersection()
		} else if cmd.Flags().Changed("union") {
			rs.union()
		} else {
			rs.difference()
		}
		l.Debug().Str("rs.operation", rs.operation).Send()
		if err := rs.printSet(); err != nil {
			l.Fatal().Err(err).Send()
		}
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolVarP(&caseSensitive, "case-sensitive", "c", false, "enable case insensitive comparison")
	rootCmd.Flags().StringVarP(&delimiter, "delimiter", "d", ",", "delimiter for CSV files, default is comma")
	rootCmd.Flags().BoolVarP(&ignoreFQDN, "ignore-fqdn", "f", false, "ignore FQDNs")
	rootCmd.Flags().BoolVarP(&pipe, "pipe", "p", false, "do not print headers to allow the output to be piped")
	rootCmd.Flags().BoolP("intersection", "i", false, "show the intersection of the two files")
	rootCmd.Flags().BoolP("union", "u", false, "show the union of the two files")
	rootCmd.MarkFlagsMutuallyExclusive("intersection", "union")
	rootCmd.PersistentFlags().CountP("verbose", "v", "verbose output")
}
