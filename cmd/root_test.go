package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alexandrestein/gods/sets/hashset"
)

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

func withFlags(t *testing.T, cs bool, delim string, ignore bool) {
	t.Helper()
	prevCase := caseSensitive
	prevDelimiter := delimiter
	prevIgnore := ignoreFQDN
	caseSensitive = cs
	delimiter = delim
	ignoreFQDN = ignore
	t.Cleanup(func() {
		caseSensitive = prevCase
		delimiter = prevDelimiter
		ignoreFQDN = prevIgnore
	})
}

func withPipe(t *testing.T, value bool) {
	t.Helper()
	prev := pipe
	pipe = value
	t.Cleanup(func() {
		pipe = prev
	})
}

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

func resetRootCmd() {
	rootCmd.ResetFlags()
	rootCmd.Flags().BoolVarP(&caseSensitive, "case-sensitive", "c", false, "enable case insensitive comparison")
	rootCmd.Flags().StringVarP(&delimiter, "delimiter", "d", ",", "delimiter for CSV files, default is comma")
	rootCmd.Flags().BoolVarP(&ignoreFQDN, "ignore-fqdn", "f", false, "ignore FQDNs")
	rootCmd.Flags().BoolVarP(&pipe, "pipe", "p", false, "do not print headers to allow the output to be piped")
	rootCmd.Flags().BoolP("intersection", "i", false, "show the intersection of the two files")
	rootCmd.Flags().BoolP("union", "u", false, "show the union of the two files")
	rootCmd.MarkFlagsMutuallyExclusive("intersection", "union")
	rootCmd.PersistentFlags().CountP("verbose", "v", "verbose output")
}

func TestFileToSetNormalization(t *testing.T) {
	withFlags(t, false, ",", true)

	path := writeTempFile(t, []string{
		" host01.example.com , web",
		"HOST02.example.com,db",
		"",
		"host01.example.com , duplicate",
		"host03.example.com",
	})

	fs := fileSet{path: path, set: *hashset.New()}
	if err := fs.fileToSet(); err != nil {
		t.Fatalf("fileToSet returned error: %v", err)
	}

	got := convertToSortedStringSlice(fs.set)
	want := []string{"host01", "host02", "host03"}
	assertStringSlice(t, got, want)
}

func TestFileToSetCaseSensitivity(t *testing.T) {
	lines := []string{"Alpha", "alpha", "ALPHA"}
	path := writeTempFile(t, lines)

	t.Run("case-sensitive", func(t *testing.T) {
		withFlags(t, true, ",", false)
		fs := fileSet{path: path, set: *hashset.New()}
		if err := fs.fileToSet(); err != nil {
			t.Fatalf("fileToSet returned error: %v", err)
		}
		if fs.set.Size() != len(lines) {
			t.Fatalf("expected %d unique entries, got %d", len(lines), fs.set.Size())
		}
	})

	t.Run("case-insensitive", func(t *testing.T) {
		withFlags(t, false, ",", false)
		fs := fileSet{path: path, set: *hashset.New()}
		if err := fs.fileToSet(); err != nil {
			t.Fatalf("fileToSet returned error: %v", err)
		}
		if fs.set.Size() != 1 {
			t.Fatalf("expected 1 unique entry when case-insensitive, got %d", fs.set.Size())
		}
		if !fs.set.Contains("alpha") {
			t.Errorf("expected normalized value 'alpha' to be present")
		}
	})
}

func makeSet(values ...string) hashset.Set {
	hs := hashset.New()
	for _, v := range values {
		hs.Add(v)
	}
	return *hs
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func makeResults(operation string, setA, setB []string) results {
	return results{
		fileSetA:  fileSet{path: "A", set: makeSet(setA...)},
		fileSetB:  fileSet{path: "B", set: makeSet(setB...)},
		setAB:     *hashset.New(),
		setBA:     *hashset.New(),
		operation: operation,
	}
}

func withCLICleanup(t *testing.T) {
	t.Helper()
	oldArgs := os.Args
	t.Cleanup(func() {
		os.Args = oldArgs
		resetRootCmd()
	})
}

func TestResultsDifference(t *testing.T) {
	withPipe(t, false)

	r := makeResults("", []string{"a", "b", "c"}, []string{"b", "c", "d"})
	r.difference()

	assertStringSlice(t, convertToSortedStringSlice(r.setAB), []string{"a"})
	assertStringSlice(t, convertToSortedStringSlice(r.setBA), []string{"d"})
}

func TestResultsUnion(t *testing.T) {
	r := makeResults("", []string{"a", "b"}, []string{"b", "c"})
	r.union()

	assertStringSlice(t, convertToSortedStringSlice(r.setAB), []string{"a", "b", "c"})
	if r.setBA.Size() != 0 {
		t.Errorf("expected secondary set to remain empty, got size %d", r.setBA.Size())
	}
}

func TestResultsIntersection(t *testing.T) {
	r := makeResults("", []string{"a", "b", "c"}, []string{"b", "c", "d"})
	r.intersection()

	assertStringSlice(t, convertToSortedStringSlice(r.setAB), []string{"b", "c"})
	if r.setBA.Size() != 0 {
		t.Errorf("expected secondary set to remain empty, got size %d", r.setBA.Size())
	}
}

func TestPrintSetOperations(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		pipe      bool
		setAB     []string
		setBA     []string
		want      string
	}{
		{
			name:      "difference-full",
			operation: "difference",
			pipe:      false,
			setAB:     []string{"alpha", "beta"},
			setBA:     []string{"gamma"},
			want:      "Difference of A - B:\nalpha\nbeta\n\nDifference of B - A:\ngamma\n",
		},
		{
			name:      "difference-pipe",
			operation: "difference",
			pipe:      true,
			setAB:     []string{"alpha", "beta"},
			setBA:     []string{"gamma"},
			want:      "alpha\nbeta\n",
		},
		{
			name:      "union-full",
			operation: "union",
			pipe:      false,
			setAB:     []string{"alpha", "beta", "gamma"},
			want:      "Union of A and B:\nalpha\nbeta\ngamma\n",
		},
		{
			name:      "union-pipe",
			operation: "union",
			pipe:      true,
			setAB:     []string{"item1", "item2"},
			want:      "item1\nitem2\n",
		},
		{
			name:      "intersection-full",
			operation: "intersection",
			pipe:      false,
			setAB:     []string{"shared"},
			want:      "Intersection of A and B:\nshared\n",
		},
		{
			name:      "intersection-pipe",
			operation: "intersection",
			pipe:      true,
			setAB:     []string{"shared"},
			want:      "shared\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withPipe(t, tt.pipe)

			r := makeResults(tt.operation, nil, nil)
			for _, v := range tt.setAB {
				r.setAB.Add(v)
			}
			for _, v := range tt.setBA {
				r.setBA.Add(v)
			}

			output := captureOutput(t, func() {
				if err := r.printSet(); err != nil {
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
	r := makeResults("bogus", nil, nil)
	if err := r.printSet(); err == nil {
		t.Fatal("expected error for invalid operation, got nil")
	}
}

func TestFileToSetDelimiters(t *testing.T) {
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
			withFlags(t, tt.caseSens, tt.delimiter, tt.ignoreFQDN)
			path := writeTempFile(t, tt.lines)

			fs := fileSet{path: path, set: *hashset.New()}
			if err := fs.fileToSet(); err != nil {
				t.Fatalf("fileToSet returned error: %v", err)
			}

			got := convertToSortedStringSlice(fs.set)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFileToSetEmptyFile(t *testing.T) {
	withFlags(t, false, ",", false)
	path := writeTempFile(t, []string{})

	fs := fileSet{path: path, set: *hashset.New()}
	if err := fs.fileToSet(); err != nil {
		t.Fatalf("fileToSet returned error on empty file: %v", err)
	}

	if fs.set.Size() != 0 {
		t.Errorf("expected empty set for empty file, got size %d", fs.set.Size())
	}
}

func TestFileToSetNonexistentFile(t *testing.T) {
	withFlags(t, false, ",", false)

	fs := fileSet{path: "/nonexistent/path/to/file.txt", set: *hashset.New()}
	err := fs.fileToSet()

	if err == nil {
		t.Fatalf("expected error for nonexistent file, got nil")
	}
	if !strings.Contains(err.Error(), "file does not exist") {
		t.Errorf("expected 'file does not exist' error, got: %v", err)
	}
}

func TestFileToSetOnlyWhitespace(t *testing.T) {
	withFlags(t, false, ",", false)
	path := writeTempFile(t, []string{"   ", "\t\t", "", "  \n  "})

	fs := fileSet{path: path, set: *hashset.New()}
	if err := fs.fileToSet(); err != nil {
		t.Fatalf("fileToSet returned error: %v", err)
	}

	if fs.set.Size() != 0 {
		t.Errorf("expected empty set for whitespace-only file, got size %d", fs.set.Size())
	}
}

func TestResultsDifferenceWithPipeMode(t *testing.T) {
	withPipe(t, true)

	r := makeResults("", []string{"a", "b", "c"}, []string{"b", "c", "d"})
	r.difference()

	assertStringSlice(t, convertToSortedStringSlice(r.setAB), []string{"a"})
	if r.setBA.Size() != 0 {
		t.Errorf("expected setBA to be empty in pipe mode, got size %d", r.setBA.Size())
	}
}

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
				assertStringSlice(t, lines, tt.exact)
			}
		})
	}
}
