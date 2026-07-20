//go:build ignore

// Package main merges multiple Go coverage profiles (mode: set) into one.
//
// `go tool cover` has no built-in merge support, so `make test-coverage-full`
// uses this script instead of the non-existent `go tool cover -merge`.
//
// Usage:
//
//	go run scripts/merge_coverage.go -o <output> <input1> <input2> [inputN...]
//
// Each input must be a coverage profile whose first line is `mode: set`.
// Blocks are keyed by (file, startLine, startCol, endLine, endCol, numStmt);
// a block is covered (count=1) in the output when any input covers it.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

// blockKey uniquely identifies a coverage block across input profiles.
type blockKey struct {
	file                                 string
	startLine, startCol, endLine, endCol int
	numStmt                              int
}

// parseBlock parses a profile line of the form
// "file.go:startLine.startCol,endLine.endCol numStmt count".
func parseBlock(line string) (blockKey, int, error) {
	var key blockKey

	lastSpace := strings.LastIndex(line, " ")
	if lastSpace < 0 {
		return key, 0, fmt.Errorf("malformed profile line: %q", line)
	}
	head := line[:lastSpace]
	var count int
	if _, err := fmt.Sscanf(line[lastSpace+1:], "%d", &count); err != nil {
		return key, 0, fmt.Errorf("invalid block count in line %q: %w", line, err)
	}

	secondSpace := strings.LastIndex(head, " ")
	if secondSpace < 0 {
		return key, 0, fmt.Errorf("malformed profile line: %q", line)
	}
	pos := head[:secondSpace]
	if _, err := fmt.Sscanf(head[secondSpace+1:], "%d", &key.numStmt); err != nil {
		return key, 0, fmt.Errorf("invalid statement count in line %q: %w", line, err)
	}

	// The file path itself may contain ':' (e.g. Windows drive letters),
	// so split the position part at the last ':'.
	colon := strings.LastIndex(pos, ":")
	if colon < 0 {
		return key, 0, fmt.Errorf("missing position range in line %q", line)
	}
	key.file = pos[:colon]
	rng := pos[colon+1:]

	var start, end string
	if comma := strings.Index(rng, ","); comma >= 0 {
		start, end = rng[:comma], rng[comma+1:]
	} else {
		start, end = rng, rng
	}
	if _, err := fmt.Sscanf(start, "%d.%d", &key.startLine, &key.startCol); err != nil {
		return key, 0, fmt.Errorf("invalid start position in line %q: %w", line, err)
	}
	if _, err := fmt.Sscanf(end, "%d.%d", &key.endLine, &key.endCol); err != nil {
		return key, 0, fmt.Errorf("invalid end position in line %q: %w", line, err)
	}
	return key, count, nil
}

// readProfile loads a mode:set coverage profile into the merged block map.
func readProfile(path string, blocks map[blockKey]bool) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open profile %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	if !scanner.Scan() {
		return fmt.Errorf("profile %s is empty", path)
	}
	if mode := strings.TrimSpace(scanner.Text()); mode != "mode: set" {
		return fmt.Errorf("profile %s has unsupported mode %q (only \"mode: set\" is supported)", path, mode)
	}

	lineNo := 1
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		key, count, err := parseBlock(line)
		if err != nil {
			return fmt.Errorf("profile %s line %d: %w", path, lineNo, err)
		}
		if count > 0 {
			blocks[key] = true
		} else if _, seen := blocks[key]; !seen {
			blocks[key] = false
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read profile %s: %w", path, err)
	}
	return nil
}

func main() {
	output := flag.String("o", "", "output profile path (required)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: go run scripts/merge_coverage.go -o <output> <input1> <input2> [inputN...]\n")
		fmt.Fprintf(os.Stderr, "Merges mode:set coverage profiles into a single union profile.\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	inputs := flag.Args()
	if *output == "" || len(inputs) < 2 {
		flag.Usage()
		os.Exit(2)
	}

	blocks := make(map[blockKey]bool)
	for _, in := range inputs {
		if err := readProfile(in, blocks); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	keys := make([]blockKey, 0, len(blocks))
	for k := range blocks {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.file != b.file {
			return a.file < b.file
		}
		if a.startLine != b.startLine {
			return a.startLine < b.startLine
		}
		return a.startCol < b.startCol
	})

	out, err := os.Create(*output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to create output %s: %v\n", *output, err)
		os.Exit(1)
	}
	defer func() { _ = out.Close() }()

	w := bufio.NewWriter(out)
	if _, err := fmt.Fprintln(w, "mode: set"); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to write output %s: %v\n", *output, err)
		os.Exit(1)
	}
	for _, k := range keys {
		count := 0
		if blocks[k] {
			count = 1
		}
		if _, err := fmt.Fprintf(w, "%s:%d.%d,%d.%d %d %d\n",
			k.file, k.startLine, k.startCol, k.endLine, k.endCol, k.numStmt, count); err != nil {
			fmt.Fprintf(os.Stderr, "error: failed to write output %s: %v\n", *output, err)
			os.Exit(1)
		}
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to flush output %s: %v\n", *output, err)
		os.Exit(1)
	}
}
