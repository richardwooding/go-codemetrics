// Command codemetrics reports per-function cyclomatic and cognitive complexity
// for Go source files.
//
// Usage:
//
//	codemetrics [flags] path...
//
// Each path is a .go file or a directory (walked recursively for .go files,
// skipping testdata and dot-directories). With no paths, source is read from
// stdin.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	codemetrics "github.com/richardwooding/go-codemetrics"
)

type row struct {
	File       string `json:"file"`
	Function   string `json:"function"`
	Cyclomatic int    `json:"cyclomatic"`
	Cognitive  *int   `json:"cognitive,omitempty"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
}

func main() {
	var (
		asJSON  = flag.Bool("json", false, "emit JSON instead of a table")
		sortKey = flag.String("sort", "cognitive", "sort key: cognitive | cyclomatic")
		top     = flag.Int("top", 0, "show only the top N rows (0 = all)")
		min     = flag.Int("min", 0, "only show functions whose sort metric is >= this")
	)
	flag.Parse()

	rows, err := collect(flag.Args())
	if err != nil {
		fmt.Fprintln(os.Stderr, "codemetrics:", err)
		os.Exit(1)
	}

	metric := func(r row) int {
		if *sortKey == "cyclomatic" || r.Cognitive == nil {
			return r.Cyclomatic
		}
		return *r.Cognitive
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if mi, mj := metric(rows[i]), metric(rows[j]); mi != mj {
			return mi > mj
		}
		if rows[i].File != rows[j].File {
			return rows[i].File < rows[j].File
		}
		return rows[i].StartLine < rows[j].StartLine
	})

	if *min > 0 {
		kept := rows[:0]
		for _, r := range rows {
			if metric(r) >= *min {
				kept = append(kept, r)
			}
		}
		rows = kept
	}
	if *top > 0 && len(rows) > *top {
		rows = rows[:*top]
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rows); err != nil {
			fmt.Fprintln(os.Stderr, "codemetrics:", err)
			os.Exit(1)
		}
		return
	}
	printTable(os.Stdout, rows)
}

// collect parses every source named by args (files, directories, or stdin
// when empty) and returns one row per function.
func collect(args []string) ([]row, error) {
	if len(args) == 0 {
		src, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
		return rowsFor("<stdin>", src)
	}
	var out []row
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			r, err := rowsForFile(arg)
			if err != nil {
				return nil, err
			}
			out = append(out, r...)
			continue
		}
		err = filepath.WalkDir(arg, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != arg && (strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata" || d.Name() == "vendor") {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			r, ferr := rowsForFile(path)
			if ferr != nil {
				return ferr
			}
			out = append(out, r...)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func rowsForFile(path string) ([]row, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return rowsFor(path, src)
}

func rowsFor(name string, src []byte) ([]row, error) {
	fns, err := codemetrics.ParseGo(src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	out := make([]row, 0, len(fns))
	for _, f := range fns {
		out = append(out, row{
			File:       name,
			Function:   f.QualifiedName(),
			Cyclomatic: f.Cyclomatic,
			Cognitive:  f.Cognitive,
			StartLine:  f.StartLine,
			EndLine:    f.EndLine,
		})
	}
	return out, nil
}

func printTable(w io.Writer, rows []row) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "COGNITIVE\tCYCLOMATIC\tLINES\tFUNCTION\tLOCATION")
	for _, r := range rows {
		cog := "-"
		if r.Cognitive != nil {
			cog = fmt.Sprintf("%d", *r.Cognitive)
		}
		lines := r.EndLine - r.StartLine + 1
		fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%s:%d\n", cog, r.Cyclomatic, lines, r.Function, r.File, r.StartLine)
	}
	_ = tw.Flush()
}
