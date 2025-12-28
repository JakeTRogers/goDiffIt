package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/emirpasic/gods/v2/sets/hashset"
)

// writeTempFile creates a temporary file with the given lines and returns its path.
// The file is automatically cleaned up when the test completes.
func writeTempFile(t *testing.T, lines []string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "input.txt")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}

// testConfig returns a config struct for testing purposes.
func testConfig(caseSens bool, delim string, ignoreFQDN, pipeMode bool) *config {
	return &config{
		caseSensitive: caseSens,
		delimiter:     delim,
		format:        "text",
		ignoreFQDN:    ignoreFQDN,
		pipe:          pipeMode,
		output:        "",
		count:         false,
		stats:         false,
		extract:       nil,
		trimPrefix:    "",
		trimSuffix:    "",
	}
}

// captureOutput captures stdout during fn execution and returns the output.
func captureOutput(t *testing.T, fn func()) string {
	t.Helper()

	prevStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = prevStdout
	})

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}
	<-done
	if err := r.Close(); err != nil {
		t.Fatalf("failed to close reader: %v", err)
	}

	return buf.String()
}

// resetRootCmd resets Cobra flags to their defaults for CLI integration tests.
func resetRootCmd() {
	rootCmd.ResetFlags()
	rootCmd.Flags().BoolVarP(&caseSensitive, "case-sensitive", "c", false, "preserve case during comparison")
	rootCmd.Flags().BoolVar(&count, "count", false, "output only the count of results instead of the elements")
	rootCmd.Flags().StringVarP(&delimiter, "delimiter", "d", ",", "delimiter for splitting lines")
	rootCmd.Flags().StringVarP(&extract, "extract", "e", "", "extract values using regex pattern")
	rootCmd.Flags().StringVar(&format, "format", "text", "output format: text, json, or csv")
	rootCmd.Flags().BoolVarP(&ignoreFQDN, "ignore-fqdn", "f", false, "strip FQDN suffixes")
	rootCmd.Flags().StringVarP(&output, "output", "o", "", "write output to file instead of stdout")
	rootCmd.Flags().BoolVarP(&pipe, "pipe", "p", false, "suppress headers for piped output")
	rootCmd.Flags().BoolVar(&stats, "stats", false, "show statistics about the file sets (size, overlap, unique elements)")
	rootCmd.Flags().StringVar(&trimPrefix, "trim-prefix", "", "remove specified prefix from each line")
	rootCmd.Flags().StringVar(&trimSuffix, "trim-suffix", "", "remove specified suffix from each line")
	rootCmd.Flags().BoolP("intersection", "i", false, "show the intersection of the two files")
	rootCmd.Flags().BoolP("union", "u", false, "show the union of the two files")
	rootCmd.Flags().BoolP("symmetric-difference", "s", false, "show the symmetric difference (XOR) of the two files")
	rootCmd.MarkFlagsMutuallyExclusive("intersection", "union", "symmetric-difference")
	rootCmd.PersistentFlags().CountP("verbose", "v", "increase verbosity")
}

// withCLICleanup saves and restores os.Args and Cobra flags for CLI tests.
// Returns a function to call to perform cleanup (also registered with t.Cleanup).
func withCLICleanup(t *testing.T) {
	t.Helper()
	oldArgs := os.Args
	t.Cleanup(func() {
		os.Args = oldArgs
		resetRootCmd()
	})
}

// makeSet creates a hashset from the given string values.
func makeSet(values ...string) *hashset.Set[string] {
	hs := hashset.New[string]()
	for _, v := range values {
		hs.Add(v)
	}
	return hs
}

// makeResults creates a results struct for testing set operations.
func makeResults(setA, setB []string) results {
	return results{
		fileSetA: fileSet{path: "A", set: makeSet(setA...)},
		fileSetB: fileSet{path: "B", set: makeSet(setB...)},
		diffAB:   hashset.New[string](),
		diffBA:   hashset.New[string](),
	}
}

// assertStringSlice compares two string slices and fails if they differ.
func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// --- fileToSet tests ---

func TestFileToSet(t *testing.T) {
	t.Parallel()

	t.Run("normalization", func(t *testing.T) {
		t.Parallel()
		cfg := testConfig(false, ",", true, false)
		path := writeTempFile(t, []string{
			" host01.example.com , web",
			"HOST02.example.com,db",
			"",
			"host01.example.com , duplicate",
			"host03.example.com",
		})

		set, err := fileToSet(path, cfg)
		if err != nil {
			t.Fatalf("fileToSet returned error: %v", err)
		}

		got := toSortedSlice(set)
		want := []string{"host01", "host02", "host03"}
		assertStringSlice(t, got, want)
	})

	t.Run("case-sensitive", func(t *testing.T) {
		t.Parallel()
		lines := []string{"Alpha", "alpha", "ALPHA"}
		path := writeTempFile(t, lines)
		cfg := testConfig(true, ",", false, false)

		set, err := fileToSet(path, cfg)
		if err != nil {
			t.Fatalf("fileToSet returned error: %v", err)
		}
		if set.Size() != len(lines) {
			t.Fatalf("expected %d unique entries, got %d", len(lines), set.Size())
		}
	})

	t.Run("case-insensitive", func(t *testing.T) {
		t.Parallel()
		path := writeTempFile(t, []string{"Alpha", "alpha", "ALPHA"})
		cfg := testConfig(false, ",", false, false)

		set, err := fileToSet(path, cfg)
		if err != nil {
			t.Fatalf("fileToSet returned error: %v", err)
		}
		if set.Size() != 1 {
			t.Fatalf("expected 1 unique entry when case-insensitive, got %d", set.Size())
		}
		if !set.Contains("alpha") {
			t.Errorf("expected normalized value 'alpha' to be present")
		}
	})

	t.Run("empty-file", func(t *testing.T) {
		t.Parallel()
		cfg := testConfig(false, ",", false, false)
		path := writeTempFile(t, []string{})

		set, err := fileToSet(path, cfg)
		if err != nil {
			t.Fatalf("fileToSet returned error on empty file: %v", err)
		}
		if set.Size() != 0 {
			t.Errorf("expected empty set for empty file, got size %d", set.Size())
		}
	})

	t.Run("nonexistent-file", func(t *testing.T) {
		t.Parallel()
		cfg := testConfig(false, ",", false, false)

		_, err := fileToSet("/nonexistent/path/to/file.txt", cfg)
		if err == nil {
			t.Fatalf("expected error for nonexistent file, got nil")
		}
		if !strings.Contains(err.Error(), "file does not exist") {
			t.Errorf("expected 'file does not exist' error, got: %v", err)
		}
	})

	t.Run("whitespace-only", func(t *testing.T) {
		t.Parallel()
		cfg := testConfig(false, ",", false, false)
		path := writeTempFile(t, []string{"   ", "\t\t", "", "  \n  "})

		set, err := fileToSet(path, cfg)
		if err != nil {
			t.Fatalf("fileToSet returned error: %v", err)
		}
		if set.Size() != 0 {
			t.Errorf("expected empty set for whitespace-only file, got size %d", set.Size())
		}
	})
}

func TestFileToSetStdin(t *testing.T) {
	// Cannot run in parallel: modifies os.Stdin

	cfg := testConfig(false, ",", false, false)

	// Create a pipe to simulate stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	// Write test data to the pipe
	go func() {
		defer func() {
			if err := w.Close(); err != nil {
				t.Logf("failed to close write pipe: %v", err)
			}
		}()
		_, _ = w.WriteString("alpha\nbeta\ngamma\n")
	}()

	// Replace stdin temporarily
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
		if err := r.Close(); err != nil {
			t.Logf("failed to close read pipe: %v", err)
		}
	}()

	set, err := fileToSet("-", cfg)
	if err != nil {
		t.Fatalf("fileToSet with stdin returned error: %v", err)
	}

	got := toSortedSlice(set)
	want := []string{"alpha", "beta", "gamma"}
	assertStringSlice(t, got, want)
}

func TestFileToSetDelimiters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		lines      []string
		delimiter  string
		ignoreFQDN bool
		caseSens   bool
		expected   []string
	}{
		{
			name:       "csv-basic",
			lines:      []string{"host1,web", "host2,db"},
			delimiter:  ",",
			ignoreFQDN: false,
			caseSens:   false,
			expected:   []string{"host1", "host2"},
		},
		{
			name:       "pipe-delimited",
			lines:      []string{"host1|web", "host2|db"},
			delimiter:  "|",
			ignoreFQDN: false,
			caseSens:   false,
			expected:   []string{"host1", "host2"},
		},
		{
			name:       "fqdn-strip",
			lines:      []string{"host1.example.com", "host2.example.com"},
			delimiter:  ",",
			ignoreFQDN: true,
			caseSens:   false,
			expected:   []string{"host1", "host2"},
		},
		{
			name:       "tab-delimited",
			lines:      []string{"host1\tweb", "host2\tdb"},
			delimiter:  "\t",
			ignoreFQDN: false,
			caseSens:   false,
			expected:   []string{"host1", "host2"},
		},
		{
			name:       "csv-with-fqdn",
			lines:      []string{"host1.corp.net,web", "host2.corp.net,db"},
			delimiter:  ",",
			ignoreFQDN: true,
			caseSens:   false,
			expected:   []string{"host1", "host2"},
		},
		{
			name:       "case-sensitive-unique",
			lines:      []string{"Host1", "host1", "HOST1"},
			delimiter:  ",",
			ignoreFQDN: false,
			caseSens:   true,
			expected:   []string{"HOST1", "Host1", "host1"},
		},
		{
			name:       "unicode-hostnames",
			lines:      []string{"服务器1,web", "сервер2,db"},
			delimiter:  ",",
			ignoreFQDN: false,
			caseSens:   false,
			expected:   []string{"сервер2", "服务器1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := testConfig(tt.caseSens, tt.delimiter, tt.ignoreFQDN, false)
			path := writeTempFile(t, tt.lines)

			set, err := fileToSet(path, cfg)
			if err != nil {
				t.Fatalf("fileToSet returned error: %v", err)
			}

			got := toSortedSlice(set)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFileToSetExtract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		lines    []string
		pattern  string
		expected []string
	}{
		{
			name:     "simple-match",
			lines:    []string{"host1", "host2", "host3"},
			pattern:  `host\d`,
			expected: []string{"host1", "host2", "host3"},
		},
		{
			name:     "capture-group",
			lines:    []string{"id=123", "id=456", "id=789"},
			pattern:  `id=(\d+)`,
			expected: []string{"123", "456", "789"},
		},
		{
			name:     "extract-hostname-from-url",
			lines:    []string{"https://foo.example.com/path", "https://bar.example.com/api"},
			pattern:  `https://([^/]+)`,
			expected: []string{"bar.example.com", "foo.example.com"},
		},
		{
			name:     "skip-non-matching",
			lines:    []string{"match123", "nomatch", "match456"},
			pattern:  `match(\d+)`,
			expected: []string{"123", "456"},
		},
		{
			name:     "full-match-no-capture",
			lines:    []string{"error: something failed", "error: another issue"},
			pattern:  `error: .*`,
			expected: []string{"error: another issue", "error: something failed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := testConfig(false, ",", false, false)
			cfg.extract = regexp.MustCompile(tt.pattern)
			path := writeTempFile(t, tt.lines)

			set, err := fileToSet(path, cfg)
			if err != nil {
				t.Fatalf("fileToSet returned error: %v", err)
			}

			got := toSortedSlice(set)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFileToSetTrimPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		lines      []string
		trimPrefix string
		trimSuffix string
		expected   []string
	}{
		{
			name:       "trim-prefix",
			lines:      []string{"prefix_alpha", "prefix_beta", "prefix_gamma"},
			trimPrefix: "prefix_",
			trimSuffix: "",
			expected:   []string{"alpha", "beta", "gamma"},
		},
		{
			name:       "trim-suffix",
			lines:      []string{"alpha_suffix", "beta_suffix", "gamma_suffix"},
			trimPrefix: "",
			trimSuffix: "_suffix",
			expected:   []string{"alpha", "beta", "gamma"},
		},
		{
			name:       "trim-both",
			lines:      []string{"[host1]", "[host2]", "[host3]"},
			trimPrefix: "[",
			trimSuffix: "]",
			expected:   []string{"host1", "host2", "host3"},
		},
		{
			name:       "no-match-prefix",
			lines:      []string{"alpha", "beta", "prefix_gamma"},
			trimPrefix: "prefix_",
			trimSuffix: "",
			expected:   []string{"alpha", "beta", "gamma"},
		},
		{
			name:       "empty-after-trim",
			lines:      []string{"prefix_", "prefix_alpha"},
			trimPrefix: "prefix_",
			trimSuffix: "",
			expected:   []string{"alpha"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := testConfig(false, ",", false, false)
			cfg.trimPrefix = tt.trimPrefix
			cfg.trimSuffix = tt.trimSuffix
			path := writeTempFile(t, tt.lines)

			set, err := fileToSet(path, cfg)
			if err != nil {
				t.Fatalf("fileToSet returned error: %v", err)
			}

			got := toSortedSlice(set)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

// --- Set operation tests ---

func TestResultsDifference(t *testing.T) {
	t.Parallel()

	t.Run("normal-mode", func(t *testing.T) {
		t.Parallel()
		cfg := testConfig(false, ",", false, false)
		r := makeResults([]string{"a", "b", "c"}, []string{"b", "c", "d"})
		r.difference(cfg)

		assertStringSlice(t, toSortedSlice(r.diffAB), []string{"a"})
		assertStringSlice(t, toSortedSlice(r.diffBA), []string{"d"})
	})

	t.Run("pipe-mode", func(t *testing.T) {
		t.Parallel()
		cfg := testConfig(false, ",", false, true)
		r := makeResults([]string{"a", "b", "c"}, []string{"b", "c", "d"})
		r.difference(cfg)

		assertStringSlice(t, toSortedSlice(r.diffAB), []string{"a"})
		if r.diffBA.Size() != 0 {
			t.Errorf("expected diffBA to be empty in pipe mode, got size %d", r.diffBA.Size())
		}
	})
}

func TestResultsUnion(t *testing.T) {
	t.Parallel()
	r := makeResults([]string{"a", "b"}, []string{"b", "c"})
	r.union()

	assertStringSlice(t, toSortedSlice(r.diffAB), []string{"a", "b", "c"})
	if r.diffBA.Size() != 0 {
		t.Errorf("expected secondary set to remain empty, got size %d", r.diffBA.Size())
	}
}

func TestResultsIntersection(t *testing.T) {
	t.Parallel()
	r := makeResults([]string{"a", "b", "c"}, []string{"b", "c", "d"})
	r.intersection()

	assertStringSlice(t, toSortedSlice(r.diffAB), []string{"b", "c"})
	if r.diffBA.Size() != 0 {
		t.Errorf("expected secondary set to remain empty, got size %d", r.diffBA.Size())
	}
}

func TestResultsSymmetricDifference(t *testing.T) {
	t.Parallel()

	t.Run("basic", func(t *testing.T) {
		t.Parallel()
		r := makeResults([]string{"a", "b", "c"}, []string{"b", "c", "d"})
		r.symmetricDifference()

		assertStringSlice(t, toSortedSlice(r.diffAB), []string{"a", "d"})
		if r.diffBA.Size() != 0 {
			t.Errorf("expected secondary set to remain empty, got size %d", r.diffBA.Size())
		}
	})

	t.Run("identical-sets", func(t *testing.T) {
		t.Parallel()
		r := makeResults([]string{"a", "b", "c"}, []string{"a", "b", "c"})
		r.symmetricDifference()

		if r.diffAB.Size() != 0 {
			t.Errorf("expected empty result for identical sets, got size %d", r.diffAB.Size())
		}
	})

	t.Run("disjoint-sets", func(t *testing.T) {
		t.Parallel()
		r := makeResults([]string{"a", "b"}, []string{"c", "d"})
		r.symmetricDifference()

		assertStringSlice(t, toSortedSlice(r.diffAB), []string{"a", "b", "c", "d"})
	})
}

// --- printSet tests ---

func TestPrintSetOperations(t *testing.T) {
	// Cannot run in parallel: captureOutput modifies global os.Stdout

	tests := []struct {
		name      string
		operation string
		pipe      bool
		diffAB    []string
		diffBA    []string
		want      string
	}{
		{
			name:      "difference-full",
			operation: "difference",
			pipe:      false,
			diffAB:    []string{"alpha", "beta"},
			diffBA:    []string{"gamma"},
			want:      "Difference of A - B:\nalpha\nbeta\n\nDifference of B - A:\ngamma\n",
		},
		{
			name:      "difference-pipe",
			operation: "difference",
			pipe:      true,
			diffAB:    []string{"alpha", "beta"},
			diffBA:    []string{"gamma"},
			want:      "alpha\nbeta\n",
		},
		{
			name:      "union-full",
			operation: "union",
			pipe:      false,
			diffAB:    []string{"alpha", "beta", "gamma"},
			want:      "Union of A and B:\nalpha\nbeta\ngamma\n",
		},
		{
			name:      "union-pipe",
			operation: "union",
			pipe:      true,
			diffAB:    []string{"item1", "item2"},
			want:      "item1\nitem2\n",
		},
		{
			name:      "intersection-full",
			operation: "intersection",
			pipe:      false,
			diffAB:    []string{"shared"},
			want:      "Intersection of A and B:\nshared\n",
		},
		{
			name:      "intersection-pipe",
			operation: "intersection",
			pipe:      true,
			diffAB:    []string{"shared"},
			want:      "shared\n",
		},
		{
			name:      "symmetric-difference-full",
			operation: "symmetric-difference",
			pipe:      false,
			diffAB:    []string{"alpha", "delta"},
			want:      "Symmetric difference of A and B:\nalpha\ndelta\n",
		},
		{
			name:      "symmetric-difference-pipe",
			operation: "symmetric-difference",
			pipe:      true,
			diffAB:    []string{"alpha", "delta"},
			want:      "alpha\ndelta\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig(false, ",", false, tt.pipe)

			r := results{
				fileSetA:  fileSet{path: "A", set: hashset.New[string]()},
				fileSetB:  fileSet{path: "B", set: hashset.New[string]()},
				diffAB:    hashset.New[string](),
				diffBA:    hashset.New[string](),
				operation: tt.operation,
			}
			for _, v := range tt.diffAB {
				r.diffAB.Add(v)
			}
			for _, v := range tt.diffBA {
				r.diffBA.Add(v)
			}

			output := captureOutput(t, func() {
				if err := r.printSet(cfg); err != nil {
					t.Fatalf("printSet returned error: %v", err)
				}
			})

			if output != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", output, tt.want)
			}
		})
	}
}

func TestPrintSetInvalidOperation(t *testing.T) {
	t.Parallel()
	cfg := testConfig(false, ",", false, false)
	r := results{
		fileSetA:  fileSet{path: "A", set: hashset.New[string]()},
		fileSetB:  fileSet{path: "B", set: hashset.New[string]()},
		diffAB:    hashset.New[string](),
		diffBA:    hashset.New[string](),
		operation: "bogus",
	}
	if err := r.printSet(cfg); err == nil {
		t.Fatal("expected error for invalid operation, got nil")
	}
}

func TestPrintSetOutputFile(t *testing.T) {
	t.Parallel()

	t.Run("write-to-file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		outPath := filepath.Join(dir, "output.txt")

		cfg := testConfig(false, ",", false, false)
		cfg.output = outPath

		r := results{
			fileSetA:  fileSet{path: "A", set: hashset.New[string]()},
			fileSetB:  fileSet{path: "B", set: hashset.New[string]()},
			diffAB:    makeSet("alpha", "beta"),
			diffBA:    hashset.New[string](),
			operation: "union",
		}

		if err := r.printSet(cfg); err != nil {
			t.Fatalf("printSet returned error: %v", err)
		}

		content, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}

		got := string(content)
		want := "Union of A and B:\nalpha\nbeta\n"
		if got != want {
			t.Errorf("got:\n%q\nwant:\n%q", got, want)
		}
	})

	t.Run("pipe-mode-to-file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		outPath := filepath.Join(dir, "output.txt")

		cfg := testConfig(false, ",", false, true)
		cfg.output = outPath

		r := results{
			fileSetA:  fileSet{path: "A", set: hashset.New[string]()},
			fileSetB:  fileSet{path: "B", set: hashset.New[string]()},
			diffAB:    makeSet("item1", "item2"),
			diffBA:    hashset.New[string](),
			operation: "intersection",
		}

		if err := r.printSet(cfg); err != nil {
			t.Fatalf("printSet returned error: %v", err)
		}

		content, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}

		got := string(content)
		want := "item1\nitem2\n"
		if got != want {
			t.Errorf("got:\n%q\nwant:\n%q", got, want)
		}
	})

	t.Run("invalid-path", func(t *testing.T) {
		t.Parallel()
		cfg := testConfig(false, ",", false, false)
		cfg.output = "/nonexistent/directory/output.txt"

		r := results{
			fileSetA:  fileSet{path: "A", set: hashset.New[string]()},
			fileSetB:  fileSet{path: "B", set: hashset.New[string]()},
			diffAB:    makeSet("test"),
			diffBA:    hashset.New[string](),
			operation: "union",
		}

		if err := r.printSet(cfg); err == nil {
			t.Fatal("expected error for invalid output path, got nil")
		}
	})
}

func TestPrintSetCountMode(t *testing.T) {
	// Cannot run in parallel: captureOutput modifies global os.Stdout

	t.Run("difference", func(t *testing.T) {
		cfg := testConfig(false, ",", false, false)
		cfg.count = true

		r := results{
			fileSetA:  fileSet{path: "A", set: hashset.New[string]()},
			fileSetB:  fileSet{path: "B", set: hashset.New[string]()},
			diffAB:    makeSet("alpha", "beta", "gamma"),
			diffBA:    makeSet("delta", "epsilon"),
			operation: "difference",
		}

		output := captureOutput(t, func() {
			if err := r.printSet(cfg); err != nil {
				t.Fatalf("printSet returned error: %v", err)
			}
		})

		want := "A-B: 3\nB-A: 2\n"
		if output != want {
			t.Errorf("got:\n%q\nwant:\n%q", output, want)
		}
	})

	t.Run("union", func(t *testing.T) {
		cfg := testConfig(false, ",", false, false)
		cfg.count = true

		r := results{
			fileSetA:  fileSet{path: "A", set: hashset.New[string]()},
			fileSetB:  fileSet{path: "B", set: hashset.New[string]()},
			diffAB:    makeSet("a", "b", "c", "d", "e"),
			diffBA:    hashset.New[string](),
			operation: "union",
		}

		output := captureOutput(t, func() {
			if err := r.printSet(cfg); err != nil {
				t.Fatalf("printSet returned error: %v", err)
			}
		})

		want := "5\n"
		if output != want {
			t.Errorf("got:\n%q\nwant:\n%q", output, want)
		}
	})

	t.Run("intersection", func(t *testing.T) {
		cfg := testConfig(false, ",", false, false)
		cfg.count = true

		r := results{
			fileSetA:  fileSet{path: "A", set: hashset.New[string]()},
			fileSetB:  fileSet{path: "B", set: hashset.New[string]()},
			diffAB:    makeSet("shared1", "shared2"),
			diffBA:    hashset.New[string](),
			operation: "intersection",
		}

		output := captureOutput(t, func() {
			if err := r.printSet(cfg); err != nil {
				t.Fatalf("printSet returned error: %v", err)
			}
		})

		want := "2\n"
		if output != want {
			t.Errorf("got:\n%q\nwant:\n%q", output, want)
		}
	})

	t.Run("symmetric-difference", func(t *testing.T) {
		cfg := testConfig(false, ",", false, false)
		cfg.count = true

		r := results{
			fileSetA:  fileSet{path: "A", set: hashset.New[string]()},
			fileSetB:  fileSet{path: "B", set: hashset.New[string]()},
			diffAB:    makeSet("unique1", "unique2", "unique3"),
			diffBA:    hashset.New[string](),
			operation: "symmetric-difference",
		}

		output := captureOutput(t, func() {
			if err := r.printSet(cfg); err != nil {
				t.Fatalf("printSet returned error: %v", err)
			}
		})

		want := "3\n"
		if output != want {
			t.Errorf("got:\n%q\nwant:\n%q", output, want)
		}
	})

	t.Run("empty-result", func(t *testing.T) {
		cfg := testConfig(false, ",", false, false)
		cfg.count = true

		r := results{
			fileSetA:  fileSet{path: "A", set: hashset.New[string]()},
			fileSetB:  fileSet{path: "B", set: hashset.New[string]()},
			diffAB:    hashset.New[string](),
			diffBA:    hashset.New[string](),
			operation: "intersection",
		}

		output := captureOutput(t, func() {
			if err := r.printSet(cfg); err != nil {
				t.Fatalf("printSet returned error: %v", err)
			}
		})

		want := "0\n"
		if output != want {
			t.Errorf("got:\n%q\nwant:\n%q", output, want)
		}
	})
}

func TestPrintSetStatsMode(t *testing.T) {
	// Cannot run in parallel: captureOutput modifies global os.Stdout

	t.Run("basic-stats", func(t *testing.T) {
		cfg := testConfig(false, ",", false, false)
		cfg.stats = true

		// Create sets where A has 5 elements, B has 4, with 3 overlapping
		setA := makeSet("a", "b", "c", "d", "e")
		setB := makeSet("b", "c", "d", "f")

		r := results{
			fileSetA: fileSet{path: "A", set: setA},
			fileSetB: fileSet{path: "B", set: setB},
			diffAB:   hashset.New[string](),
			diffBA:   hashset.New[string](),
		}

		output := captureOutput(t, func() {
			if err := r.printSet(cfg); err != nil {
				t.Fatalf("printSet returned error: %v", err)
			}
		})

		// Check for expected output
		if !strings.Contains(output, "File A: 5 unique lines") {
			t.Errorf("expected 'File A: 5 unique lines', got: %q", output)
		}
		if !strings.Contains(output, "File B: 4 unique lines") {
			t.Errorf("expected 'File B: 4 unique lines', got: %q", output)
		}
		if !strings.Contains(output, "Overlap: 3") {
			t.Errorf("expected 'Overlap: 3', got: %q", output)
		}
		if !strings.Contains(output, "60.0% of A") {
			t.Errorf("expected '60.0%% of A', got: %q", output)
		}
		if !strings.Contains(output, "75.0% of B") {
			t.Errorf("expected '75.0%% of B', got: %q", output)
		}
		if !strings.Contains(output, "Only in A: 2") {
			t.Errorf("expected 'Only in A: 2', got: %q", output)
		}
		if !strings.Contains(output, "Only in B: 1") {
			t.Errorf("expected 'Only in B: 1', got: %q", output)
		}
	})

	t.Run("no-overlap", func(t *testing.T) {
		cfg := testConfig(false, ",", false, false)
		cfg.stats = true

		setA := makeSet("a", "b")
		setB := makeSet("c", "d")

		r := results{
			fileSetA: fileSet{path: "A", set: setA},
			fileSetB: fileSet{path: "B", set: setB},
			diffAB:   hashset.New[string](),
			diffBA:   hashset.New[string](),
		}

		output := captureOutput(t, func() {
			if err := r.printSet(cfg); err != nil {
				t.Fatalf("printSet returned error: %v", err)
			}
		})

		if !strings.Contains(output, "Overlap: 0 (0.0% of A, 0.0% of B)") {
			t.Errorf("expected no overlap with percentages, got: %q", output)
		}
		if !strings.Contains(output, "Only in A: 2") {
			t.Errorf("expected 'Only in A: 2', got: %q", output)
		}
		if !strings.Contains(output, "Only in B: 2") {
			t.Errorf("expected 'Only in B: 2', got: %q", output)
		}
	})

	t.Run("complete-overlap", func(t *testing.T) {
		cfg := testConfig(false, ",", false, false)
		cfg.stats = true

		setA := makeSet("a", "b", "c")
		setB := makeSet("a", "b", "c")

		r := results{
			fileSetA: fileSet{path: "A", set: setA},
			fileSetB: fileSet{path: "B", set: setB},
			diffAB:   hashset.New[string](),
			diffBA:   hashset.New[string](),
		}

		output := captureOutput(t, func() {
			if err := r.printSet(cfg); err != nil {
				t.Fatalf("printSet returned error: %v", err)
			}
		})

		if !strings.Contains(output, "Overlap: 3 (100.0% of A, 100.0% of B)") {
			t.Errorf("expected complete overlap, got: %q", output)
		}
		if !strings.Contains(output, "Only in A: 0") {
			t.Errorf("expected 'Only in A: 0', got: %q", output)
		}
		if !strings.Contains(output, "Only in B: 0") {
			t.Errorf("expected 'Only in B: 0', got: %q", output)
		}
	})

	t.Run("empty-files", func(t *testing.T) {
		cfg := testConfig(false, ",", false, false)
		cfg.stats = true

		setA := hashset.New[string]()
		setB := hashset.New[string]()

		r := results{
			fileSetA: fileSet{path: "A", set: setA},
			fileSetB: fileSet{path: "B", set: setB},
			diffAB:   hashset.New[string](),
			diffBA:   hashset.New[string](),
		}

		output := captureOutput(t, func() {
			if err := r.printSet(cfg); err != nil {
				t.Fatalf("printSet returned error: %v", err)
			}
		})

		if !strings.Contains(output, "File A: 0 unique lines") {
			t.Errorf("expected 'File A: 0 unique lines', got: %q", output)
		}
		if !strings.Contains(output, "File B: 0 unique lines") {
			t.Errorf("expected 'File B: 0 unique lines', got: %q", output)
		}
		if !strings.Contains(output, "Overlap: 0") {
			t.Errorf("expected 'Overlap: 0', got: %q", output)
		}
	})
}

func TestPrintSetJSONFormat(t *testing.T) {
	t.Run("intersection-json", func(t *testing.T) {
		cfg := testConfig(false, ",", false, false)
		cfg.format = "json"

		r := makeResults([]string{"a", "b", "c"}, []string{"b", "c", "d"})
		r.intersection()

		output := captureOutput(t, func() {
			if err := r.printSet(cfg); err != nil {
				t.Fatalf("printSet returned error: %v", err)
			}
		})

		if !strings.Contains(output, `"operation": "intersection"`) {
			t.Errorf("expected operation field in JSON, got: %q", output)
		}
		if !strings.Contains(output, `"b"`) || !strings.Contains(output, `"c"`) {
			t.Errorf("expected b and c in results, got: %q", output)
		}
	})

	t.Run("difference-json", func(t *testing.T) {
		cfg := testConfig(false, ",", false, false)
		cfg.format = "json"

		r := makeResults([]string{"a", "b", "c"}, []string{"b", "c", "d"})
		r.difference(cfg)

		output := captureOutput(t, func() {
			if err := r.printSet(cfg); err != nil {
				t.Fatalf("printSet returned error: %v", err)
			}
		})

		if !strings.Contains(output, `"operation": "difference"`) {
			t.Errorf("expected operation field in JSON, got: %q", output)
		}
		if !strings.Contains(output, `"A-B"`) || !strings.Contains(output, `"B-A"`) {
			t.Errorf("expected A-B and B-A keys in JSON, got: %q", output)
		}
	})
}

func TestPrintSetCSVFormat(t *testing.T) {
	t.Run("union-csv", func(t *testing.T) {
		cfg := testConfig(false, ",", false, false)
		cfg.format = "csv"

		r := makeResults([]string{"a", "b"}, []string{"b", "c"})
		r.union()

		output := captureOutput(t, func() {
			if err := r.printSet(cfg); err != nil {
				t.Fatalf("printSet returned error: %v", err)
			}
		})

		lines := strings.Split(strings.TrimSpace(output), "\n")
		if lines[0] != "value" {
			t.Errorf("expected CSV header 'value', got: %q", lines[0])
		}
		// Should have a, b, c
		if len(lines) != 4 {
			t.Errorf("expected 4 lines (header + 3 values), got %d", len(lines))
		}
	})

	t.Run("difference-csv", func(t *testing.T) {
		cfg := testConfig(false, ",", false, false)
		cfg.format = "csv"

		r := makeResults([]string{"a", "b", "c"}, []string{"b", "c", "d"})
		r.difference(cfg)

		output := captureOutput(t, func() {
			if err := r.printSet(cfg); err != nil {
				t.Fatalf("printSet returned error: %v", err)
			}
		})

		if !strings.Contains(output, "set,value") {
			t.Errorf("expected CSV header 'set,value', got: %q", output)
		}
		if !strings.Contains(output, "A-B,a") {
			t.Errorf("expected 'A-B,a' in CSV, got: %q", output)
		}
		if !strings.Contains(output, "B-A,d") {
			t.Errorf("expected 'B-A,d' in CSV, got: %q", output)
		}
	})
}

// --- toSortedSlice tests ---

func TestToSortedSlice(t *testing.T) {
	t.Parallel()
	hs := hashset.New[string]()
	hs.Add("zebra")
	hs.Add("apple")
	hs.Add("mango")

	got := toSortedSlice(hs)
	want := []string{"apple", "mango", "zebra"}
	assertStringSlice(t, got, want)
}

func TestToSortedSliceEmpty(t *testing.T) {
	t.Parallel()
	hs := hashset.New[string]()
	got := toSortedSlice(hs)
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

// --- CLI integration tests ---
// These tests must NOT run in parallel as they modify global state (os.Args, flags).

var cliMu sync.Mutex

func TestCLIIntegration(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		fileA    []string
		fileB    []string
		contains []string
		excludes []string
		exact    []string
	}{
		{
			name:     "difference-pipe",
			args:     []string{"--pipe"},
			fileA:    []string{"alpha", "beta", "gamma"},
			fileB:    []string{"beta", "gamma", "delta"},
			contains: []string{"alpha"},
			excludes: []string{"delta"},
		},
		{
			name:  "union-pipe",
			args:  []string{"--union", "--pipe"},
			fileA: []string{"alpha", "beta"},
			fileB: []string{"beta", "gamma"},
			exact: []string{"alpha", "beta", "gamma"},
		},
		{
			name:  "intersection-pipe",
			args:  []string{"--intersection", "--pipe"},
			fileA: []string{"alpha", "beta", "gamma"},
			fileB: []string{"beta", "gamma", "delta"},
			exact: []string{"beta", "gamma"},
		},
		{
			name:     "ignore-fqdn",
			args:     []string{"--ignore-fqdn", "--pipe"},
			fileA:    []string{"host1.example.com", "host2.example.com"},
			fileB:    []string{"host2", "host3"},
			contains: []string{"host1"},
			excludes: []string{"host1.example.com"},
		},
		{
			name:  "case-sensitive",
			args:  []string{"--case-sensitive", "--pipe"},
			fileA: []string{"Alpha", "beta"},
			fileB: []string{"alpha", "beta"},
			exact: []string{"Alpha"},
		},
		{
			name:  "custom-delimiter",
			args:  []string{"--delimiter", "|", "--pipe"},
			fileA: []string{"host1|web", "host2|db"},
			fileB: []string{"host2", "host3"},
			exact: []string{"host1"},
		},
		{
			name:  "symmetric-difference-pipe",
			args:  []string{"--symmetric-difference", "--pipe"},
			fileA: []string{"alpha", "beta", "gamma"},
			fileB: []string{"beta", "gamma", "delta"},
			exact: []string{"alpha", "delta"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cliMu.Lock()
			defer cliMu.Unlock()

			withCLICleanup(t)

			pathA := writeTempFile(t, tt.fileA)
			pathB := writeTempFile(t, tt.fileB)

			os.Args = append([]string{"goDiffIt"}, append(tt.args, pathA, pathB)...)

			output := captureOutput(t, func() {
				Execute()
			})

			for _, want := range tt.contains {
				if !strings.Contains(output, want) {
					t.Errorf("expected output to contain %q, got:\n%s", want, output)
				}
			}

			for _, exclude := range tt.excludes {
				if strings.Contains(output, exclude) {
					t.Errorf("expected output to NOT contain %q, but got:\n%s", exclude, output)
				}
			}

			if len(tt.exact) > 0 {
				lines := strings.Split(strings.TrimSpace(output), "\n")
				sort.Strings(lines)
				want := make([]string, len(tt.exact))
				copy(want, tt.exact)
				sort.Strings(want)
				assertStringSlice(t, lines, want)
			}
		})
	}
}

func TestCLIOutputFile(t *testing.T) {
	cliMu.Lock()
	defer cliMu.Unlock()

	withCLICleanup(t)

	fileA := writeTempFile(t, []string{"alpha", "beta", "gamma"})
	fileB := writeTempFile(t, []string{"beta", "gamma", "delta"})
	outPath := filepath.Join(t.TempDir(), "result.txt")

	os.Args = []string{"goDiffIt", "--union", "--pipe", "--output", outPath, fileA, fileB}

	// Should have no output to stdout
	output := captureOutput(t, func() {
		Execute()
	})

	if output != "" {
		t.Errorf("expected no stdout output when using --output, got: %q", output)
	}

	// Check file was created and contains expected content
	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	sort.Strings(lines)
	want := []string{"alpha", "beta", "delta", "gamma"}
	assertStringSlice(t, lines, want)
}

func TestCLICountMode(t *testing.T) {
	t.Run("difference", func(t *testing.T) {
		cliMu.Lock()
		defer cliMu.Unlock()

		withCLICleanup(t)

		fileA := writeTempFile(t, []string{"alpha", "beta", "gamma"})
		fileB := writeTempFile(t, []string{"beta", "gamma", "delta"})

		os.Args = []string{"goDiffIt", "--count", fileA, fileB}

		output := captureOutput(t, func() {
			Execute()
		})

		if !strings.Contains(output, "A-B: 1") {
			t.Errorf("expected 'A-B: 1', got: %q", output)
		}
		if !strings.Contains(output, "B-A: 1") {
			t.Errorf("expected 'B-A: 1', got: %q", output)
		}
	})

	t.Run("union", func(t *testing.T) {
		cliMu.Lock()
		defer cliMu.Unlock()

		withCLICleanup(t)

		fileA := writeTempFile(t, []string{"alpha", "beta", "gamma"})
		fileB := writeTempFile(t, []string{"beta", "gamma", "delta"})

		os.Args = []string{"goDiffIt", "--union", "--count", fileA, fileB}

		output := captureOutput(t, func() {
			Execute()
		})

		if strings.TrimSpace(output) != "4" {
			t.Errorf("expected '4', got: %q", output)
		}
	})

	t.Run("intersection", func(t *testing.T) {
		cliMu.Lock()
		defer cliMu.Unlock()

		withCLICleanup(t)

		fileA := writeTempFile(t, []string{"alpha", "beta", "gamma"})
		fileB := writeTempFile(t, []string{"beta", "gamma", "delta"})

		os.Args = []string{"goDiffIt", "--intersection", "--count", fileA, fileB}

		output := captureOutput(t, func() {
			Execute()
		})

		if strings.TrimSpace(output) != "2" {
			t.Errorf("expected '2', got: %q", output)
		}
	})
}

func TestCLIStatsMode(t *testing.T) {
	cliMu.Lock()
	defer cliMu.Unlock()

	withCLICleanup(t)

	// Create test files: A has 3 elements, B has 3 elements, 2 overlapping
	fileA := writeTempFile(t, []string{"alpha", "beta", "gamma"})
	fileB := writeTempFile(t, []string{"beta", "gamma", "delta"})

	os.Args = []string{"goDiffIt", "--stats", fileA, fileB}

	output := captureOutput(t, func() {
		Execute()
	})

	// Verify expected statistics
	if !strings.Contains(output, "File A: 3 unique lines") {
		t.Errorf("expected 'File A: 3 unique lines', got: %q", output)
	}
	if !strings.Contains(output, "File B: 3 unique lines") {
		t.Errorf("expected 'File B: 3 unique lines', got: %q", output)
	}
	if !strings.Contains(output, "Overlap: 2") {
		t.Errorf("expected 'Overlap: 2', got: %q", output)
	}
	if !strings.Contains(output, "66.7% of A") {
		t.Errorf("expected '66.7%% of A', got: %q", output)
	}
	if !strings.Contains(output, "66.7% of B") {
		t.Errorf("expected '66.7%% of B', got: %q", output)
	}
	if !strings.Contains(output, "Only in A: 1") {
		t.Errorf("expected 'Only in A: 1', got: %q", output)
	}
	if !strings.Contains(output, "Only in B: 1") {
		t.Errorf("expected 'Only in B: 1', got: %q", output)
	}
}

func TestCLIExtractMode(t *testing.T) {
	cliMu.Lock()
	defer cliMu.Unlock()

	withCLICleanup(t)

	// Create test files with extractable data
	fileA := writeTempFile(t, []string{"id=123", "id=456", "id=789"})
	fileB := writeTempFile(t, []string{"id=456", "id=789", "id=999"})

	os.Args = []string{"goDiffIt", "--extract", `id=(\d+)`, "-p", fileA, fileB}

	output := captureOutput(t, func() {
		Execute()
	})

	// Should show "123" (extracted from id=123, only in A)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 || lines[0] != "123" {
		t.Errorf("expected single line '123', got: %q", output)
	}
}

func TestCLIExtractInvalidRegex(t *testing.T) {
	cliMu.Lock()
	defer cliMu.Unlock()

	withCLICleanup(t)

	fileA := writeTempFile(t, []string{"test"})
	fileB := writeTempFile(t, []string{"test"})

	// Invalid regex pattern (unclosed bracket)
	os.Args = []string{"goDiffIt", "--extract", "[invalid", fileA, fileB}

	// Capture stderr to check for error message
	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error for invalid regex, got nil")
	}
	if !strings.Contains(err.Error(), "invalid extract regex") {
		t.Errorf("expected 'invalid extract regex' error, got: %v", err)
	}
}

func TestCLITrimPatterns(t *testing.T) {
	cliMu.Lock()
	defer cliMu.Unlock()

	withCLICleanup(t)

	// Create test files with prefixed data
	fileA := writeTempFile(t, []string{"prefix_alpha", "prefix_beta", "prefix_gamma"})
	fileB := writeTempFile(t, []string{"prefix_beta", "prefix_gamma", "prefix_delta"})

	os.Args = []string{"goDiffIt", "--trim-prefix", "prefix_", "-p", fileA, fileB}

	output := captureOutput(t, func() {
		Execute()
	})

	// Should show "alpha" (only in A after trimming prefix)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 || lines[0] != "alpha" {
		t.Errorf("expected single line 'alpha', got: %q", output)
	}
}

func TestCLIFormatJSON(t *testing.T) {
	cliMu.Lock()
	defer cliMu.Unlock()

	withCLICleanup(t)

	fileA := writeTempFile(t, []string{"alpha", "beta", "gamma"})
	fileB := writeTempFile(t, []string{"beta", "gamma", "delta"})

	os.Args = []string{"goDiffIt", "--format", "json", "-i", fileA, fileB}

	output := captureOutput(t, func() {
		Execute()
	})

	// Verify it's valid JSON and contains expected fields
	if !strings.Contains(output, `"operation": "intersection"`) {
		t.Errorf("expected JSON with operation field, got: %q", output)
	}
	if !strings.Contains(output, `"beta"`) {
		t.Errorf("expected JSON with 'beta' in results, got: %q", output)
	}
	if !strings.Contains(output, `"gamma"`) {
		t.Errorf("expected JSON with 'gamma' in results, got: %q", output)
	}
}

func TestCLIFormatCSV(t *testing.T) {
	cliMu.Lock()
	defer cliMu.Unlock()

	withCLICleanup(t)

	fileA := writeTempFile(t, []string{"alpha", "beta", "gamma"})
	fileB := writeTempFile(t, []string{"beta", "gamma", "delta"})

	os.Args = []string{"goDiffIt", "--format", "csv", fileA, fileB}

	output := captureOutput(t, func() {
		Execute()
	})

	// Verify CSV format with header and data
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines (header + data), got %d", len(lines))
	}
	if lines[0] != "set,value" {
		t.Errorf("expected CSV header 'set,value', got: %q", lines[0])
	}
	// Should have A-B entry for "alpha"
	if !strings.Contains(output, "A-B,alpha") {
		t.Errorf("expected 'A-B,alpha' in CSV output, got: %q", output)
	}
	// Should have B-A entry for "delta"
	if !strings.Contains(output, "B-A,delta") {
		t.Errorf("expected 'B-A,delta' in CSV output, got: %q", output)
	}
}
