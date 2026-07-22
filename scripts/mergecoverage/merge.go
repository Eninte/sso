// Package mergecoverage 合并多个 Go 覆盖率 profile。
//
// `go tool cover` 没有内置 merge 子命令，本包提供等价能力，
// 供 scripts/merge_coverage.go（//go:build ignore）调用。
//
// 支持的 profile 模式（与 go test -coverprofile 生成的一致）：
//   - set:    块命中为 1，未命中为 0；合并语义为 OR
//   - count:  块命中次数；合并语义为 SUM
//   - atomic: 与 count 同语义（race 检测器强制使用此模式，count 仍为整数）
//
// 所有输入 profile 必须共享同一模式；输出保留该模式。
// go test -race 会强制生成 mode: atomic，因此必须支持 atomic 才能让
// Makefile 的 test-coverage-full 目标在 -race 下正常合并。
package mergecoverage

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// SupportedModes 列出本包接受的 profile 模式。
var SupportedModes = map[string]bool{
	"set":    true,
	"count":  true,
	"atomic": true,
}

// BlockKey 唯一标识一个覆盖率块，跨多个 profile 间一致。
type BlockKey struct {
	File                                 string
	StartLine, StartCol, EndLine, EndCol int
	NumStmt                              int
}

// Profile 是已解析的覆盖率 profile。
type Profile struct {
	Mode   string
	Blocks map[BlockKey]int
}

// ParseProfile 从任意 io.Reader 解析覆盖率 profile。
//
// 第一行必须为 `mode: <set|count|atomic>`。后续每行格式为：
//
//	file.go:startLine.startCol,endLine.endCol numStmt count
func ParseProfile(r io.Reader) (*Profile, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("read mode line: %w", err)
		}
		return nil, fmt.Errorf("profile is empty")
	}

	mode := strings.TrimSpace(scanner.Text())
	parts := strings.SplitN(mode, ":", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) != "mode" {
		return nil, fmt.Errorf("invalid mode line %q (expected \"mode: <mode>\")", mode)
	}
	modeName := strings.TrimSpace(parts[1])
	if !SupportedModes[modeName] {
		return nil, fmt.Errorf("unsupported mode %q (supported: set, count, atomic)", modeName)
	}

	p := &Profile{
		Mode:   modeName,
		Blocks: make(map[BlockKey]int),
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
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		// 同 profile 内重复块（理论上不应发生）取最大值，保证幂等。
		// 注意：count == 0 仍需写入 map，否则未命中块会丢失，下游合并阶段会缺数据。
		if existing, has := p.Blocks[key]; !has || count > existing {
			p.Blocks[key] = count
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}
	return p, nil
}

// ParseProfileFile 打开文件并解析为 Profile。
func ParseProfileFile(path string) (*Profile, error) {
	f, err := os.Open(path) // #nosec G304 -- CLI 工具，路径来自命令行参数，调用者即本机用户
	if err != nil {
		return nil, fmt.Errorf("failed to open profile %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	return ParseProfile(f)
}

// Merge 将多个 Profile 合并为一个。
//
// 所有输入必须共享同一模式，否则返回错误。输出保留该模式。
// 合并语义：
//   - set:    任意输入命中则输出 1
//   - count:  输出为各输入计数之和
//   - atomic: 同 count
func Merge(profiles []*Profile) (*Profile, error) {
	if len(profiles) == 0 {
		return nil, fmt.Errorf("no profiles to merge")
	}
	mode := profiles[0].Mode
	for i, p := range profiles[1:] {
		if p.Mode != mode {
			return nil, fmt.Errorf("profile %d has mode %q, expected %q (all inputs must share the same mode)", i+1, p.Mode, mode)
		}
	}

	merged := &Profile{
		Mode:   mode,
		Blocks: make(map[BlockKey]int),
	}
	for _, p := range profiles {
		for key, count := range p.Blocks {
			merged.Blocks[key] = mergeCount(mode, merged.Blocks[key], count)
		}
	}
	return merged, nil
}

// mergeCount 根据模式累加两个计数值。
func mergeCount(mode string, a, b int) int {
	switch mode {
	case "set":
		if a > 0 || b > 0 {
			return 1
		}
		return 0
	case "count", "atomic":
		return a + b
	default:
		// 不应发生：ParseProfile 已校验模式。
		return a + b
	}
}

// WriteProfile 将 Profile 写入 io.Writer，格式与 go tool cover 兼容。
func WriteProfile(w io.Writer, p *Profile) error {
	keys := make([]BlockKey, 0, len(p.Blocks))
	for k := range p.Blocks {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.StartLine != b.StartLine {
			return a.StartLine < b.StartLine
		}
		return a.StartCol < b.StartCol
	})

	bw := bufio.NewWriter(w)
	if _, err := fmt.Fprintf(bw, "mode: %s\n", p.Mode); err != nil {
		return fmt.Errorf("write mode: %w", err)
	}
	for _, k := range keys {
		count := p.Blocks[k]
		if p.Mode == "set" {
			// set 模式规范化为 0/1
			if count > 0 {
				count = 1
			}
		}
		if _, err := fmt.Fprintf(bw, "%s:%d.%d,%d.%d %d %d\n",
			k.File, k.StartLine, k.StartCol, k.EndLine, k.EndCol, k.NumStmt, count); err != nil {
			return fmt.Errorf("write block: %w", err)
		}
	}
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("flush: %w", err)
	}
	return nil
}

// WriteProfileFile 创建文件并写入 Profile。
func WriteProfileFile(path string, p *Profile) error {
	f, err := os.Create(path) // #nosec G304 -- CLI 工具，输出路径来自 -o 命令行参数，调用者即本机用户
	if err != nil {
		return fmt.Errorf("failed to create output %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	return WriteProfile(f, p)
}

// parseBlock 解析单行 "file.go:startLine.startCol,endLine.endCol numStmt count"。
//
// 文件路径本身可能包含 ':'（如 Windows 盘符），因此在最后一个 ':' 处切分。
func parseBlock(line string) (BlockKey, int, error) {
	var key BlockKey

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
	if _, err := fmt.Sscanf(head[secondSpace+1:], "%d", &key.NumStmt); err != nil {
		return key, 0, fmt.Errorf("invalid statement count in line %q: %w", line, err)
	}

	colon := strings.LastIndex(pos, ":")
	if colon < 0 {
		return key, 0, fmt.Errorf("missing position range in line %q", line)
	}
	key.File = pos[:colon]
	rng := pos[colon+1:]

	var start, end string
	if comma := strings.Index(rng, ","); comma >= 0 {
		start, end = rng[:comma], rng[comma+1:]
	} else {
		start, end = rng, rng
	}
	if _, err := fmt.Sscanf(start, "%d.%d", &key.StartLine, &key.StartCol); err != nil {
		return key, 0, fmt.Errorf("invalid start position in line %q: %w", line, err)
	}
	if _, err := fmt.Sscanf(end, "%d.%d", &key.EndLine, &key.EndCol); err != nil {
		return key, 0, fmt.Errorf("invalid end position in line %q: %w", line, err)
	}
	return key, count, nil
}
