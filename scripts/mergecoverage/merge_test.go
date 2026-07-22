// Package mergecoverage 单元测试
//
// 覆盖 ParseProfile / Merge / WriteProfile 的关键路径：
//   - 三种模式（set/count/atomic）的解析与合并语义
//   - 模式不一致拒绝合并
//   - 不支持的模式拒绝解析
//   - 空输入、空 profile、Windows 路径含 ':'
//   - 写出后再解析的 round-trip 一致性
package mergecoverage

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// block 构造一个 profile 行（无尾随换行）。
func block(file string, sl, sc, el, ec, ns, c int) string {
	return file + ":" +
		strconv.Itoa(sl) + "." + strconv.Itoa(sc) + "," + strconv.Itoa(el) + "." + strconv.Itoa(ec) +
		" " + strconv.Itoa(ns) + " " + strconv.Itoa(c)
}

// profile 拼接一个完整 profile 字符串。
func profile(mode string, lines ...string) string {
	return "mode: " + mode + "\n" + strings.Join(lines, "\n") + "\n"
}

// TestParseProfile_SupportedModes 验证三种模式均能解析。
func TestParseProfile_SupportedModes(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"set", "count", "atomic"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			in := profile(mode,
				block("foo.go", 1, 2, 3, 4, 5, 1),
				block("foo.go", 6, 7, 8, 9, 10, 0),
			)
			p, err := ParseProfile(strings.NewReader(in))
			require.NoError(t, err)
			require.NotNil(t, p)
			assert.Equal(t, mode, p.Mode)
			assert.Len(t, p.Blocks, 2, "应解析出 2 个块")
		})
	}
}

// TestParseProfile_UnsupportedMode 验证不支持的 mode 被拒绝。
func TestParseProfile_UnsupportedMode(t *testing.T) {
	t.Parallel()
	in := profile("verbatim", block("foo.go", 1, 1, 1, 1, 1, 1))
	_, err := ParseProfile(strings.NewReader(in))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported mode")
}

// TestParseProfile_InvalidModeLine 验证首行格式错误被拒绝。
func TestParseProfile_InvalidModeLine(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"missing colon": "hello world\n",
		"missing value": "mode\n",
		"wrong prefix":  "modefoo: set\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseProfile(strings.NewReader(in))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "mode")
		})
	}
}

// TestParseProfile_EmptyInputSeparately 验证空输入报独立错误（不混入 mode 错误断言）。
func TestParseProfile_EmptyInputSeparately(t *testing.T) {
	t.Parallel()
	_, err := ParseProfile(strings.NewReader(""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

// TestParseProfile_MalformedBlock 验证块行格式错误被拒绝并附带行号。
func TestParseProfile_MalformedBlock(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		line string
	}{
		{"missing count", "foo.go:1.1,2.2 5"},
		{"missing numStmt", "foo.go:1.1,2.2"},
		{"missing range", "foo.go 5 1"},
		{"missing start col", "foo.go:1,2.2 5 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			in := "mode: set\n" + c.line + "\n"
			_, err := ParseProfile(strings.NewReader(in))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "line 2")
		})
	}
}

// TestParseProfile_WindowsPath 验证文件路径含 ':'（Windows 盘符）时仍能正确解析。
func TestParseProfile_WindowsPath(t *testing.T) {
	t.Parallel()
	in := profile("set",
		block("c:/dev/sso/foo.go", 1, 2, 3, 4, 5, 1),
	)
	p, err := ParseProfile(strings.NewReader(in))
	require.NoError(t, err)
	require.Len(t, p.Blocks, 1)
	for k := range p.Blocks {
		assert.Equal(t, "c:/dev/sso/foo.go", k.File)
	}
}

// TestParseProfile_EmptyInput 验证空输入被拒绝。
func TestParseProfile_EmptyInput(t *testing.T) {
	t.Parallel()
	_, err := ParseProfile(strings.NewReader(""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

// TestParseProfile_BlankLines 验证空行被跳过。
func TestParseProfile_BlankLines(t *testing.T) {
	t.Parallel()
	in := "mode: set\n\n" + block("foo.go", 1, 1, 1, 1, 1, 1) + "\n\n\n"
	p, err := ParseProfile(strings.NewReader(in))
	require.NoError(t, err)
	assert.Len(t, p.Blocks, 1)
}

// TestMerge_ModeMismatch 验证模式不一致拒绝合并。
func TestMerge_ModeMismatch(t *testing.T) {
	t.Parallel()
	p1 := &Profile{Mode: "set", Blocks: map[BlockKey]int{}}
	p2 := &Profile{Mode: "atomic", Blocks: map[BlockKey]int{}}
	_, err := Merge([]*Profile{p1, p2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected \"set\"")
}

// TestMerge_EmptyInput 验证空输入拒绝合并。
func TestMerge_EmptyInput(t *testing.T) {
	t.Parallel()
	_, err := Merge(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no profiles")
}

// TestMerge_Set 验证 set 模式合并语义（OR）。
func TestMerge_Set(t *testing.T) {
	t.Parallel()
	k := BlockKey{File: "foo.go", StartLine: 1, StartCol: 1, EndLine: 2, EndCol: 2, NumStmt: 5}
	p1 := &Profile{Mode: "set", Blocks: map[BlockKey]int{k: 1}}
	p2 := &Profile{Mode: "set", Blocks: map[BlockKey]int{k: 0}} // 第二个未命中
	merged, err := Merge([]*Profile{p1, p2})
	require.NoError(t, err)
	assert.Equal(t, 1, merged.Blocks[k], "set: 任意命中即应为 1")

	p3 := &Profile{Mode: "set", Blocks: map[BlockKey]int{k: 0}}
	p4 := &Profile{Mode: "set", Blocks: map[BlockKey]int{k: 0}}
	merged, err = Merge([]*Profile{p3, p4})
	require.NoError(t, err)
	assert.Equal(t, 0, merged.Blocks[k], "set: 全未命中应为 0")
}

// TestMerge_Count 验证 count 模式合并语义（SUM）。
func TestMerge_Count(t *testing.T) {
	t.Parallel()
	k := BlockKey{File: "foo.go", StartLine: 1, StartCol: 1, EndLine: 2, EndCol: 2, NumStmt: 5}
	p1 := &Profile{Mode: "count", Blocks: map[BlockKey]int{k: 3}}
	p2 := &Profile{Mode: "count", Blocks: map[BlockKey]int{k: 5}}
	merged, err := Merge([]*Profile{p1, p2})
	require.NoError(t, err)
	assert.Equal(t, 8, merged.Blocks[k], "count: 应为 SUM")
}

// TestMerge_Atomic 验证 atomic 模式合并语义（与 count 一致，SUM）。
// 这是 P1 的核心回归点：之前脚本只接受 mode: set，-race 生成 atomic 时直接报错。
func TestMerge_Atomic(t *testing.T) {
	t.Parallel()
	k := BlockKey{File: "foo.go", StartLine: 1, StartCol: 1, EndLine: 2, EndCol: 2, NumStmt: 5}
	p1 := &Profile{Mode: "atomic", Blocks: map[BlockKey]int{k: 7}}
	p2 := &Profile{Mode: "atomic", Blocks: map[BlockKey]int{k: 11}}
	merged, err := Merge([]*Profile{p1, p2})
	require.NoError(t, err)
	assert.Equal(t, "atomic", merged.Mode, "atomic: 输出应保留 atomic 模式")
	assert.Equal(t, 18, merged.Blocks[k], "atomic: 应为 SUM（与 count 同）")
}

// TestMerge_UnionOfBlocks 验证不同 profile 各自独占的块在合并后都被保留。
func TestMerge_UnionOfBlocks(t *testing.T) {
	t.Parallel()
	k1 := BlockKey{File: "foo.go", StartLine: 1, StartCol: 1, EndLine: 2, EndCol: 2, NumStmt: 5}
	k2 := BlockKey{File: "bar.go", StartLine: 10, StartCol: 1, EndLine: 11, EndCol: 2, NumStmt: 3}
	p1 := &Profile{Mode: "count", Blocks: map[BlockKey]int{k1: 4}}
	p2 := &Profile{Mode: "count", Blocks: map[BlockKey]int{k2: 9}}
	merged, err := Merge([]*Profile{p1, p2})
	require.NoError(t, err)
	assert.Len(t, merged.Blocks, 2, "应保留并集")
	assert.Equal(t, 4, merged.Blocks[k1])
	assert.Equal(t, 9, merged.Blocks[k2])
}

// TestWriteProfile_RoundTrip 验证写出的 profile 可被重新解析，且数据一致。
func TestWriteProfile_RoundTrip(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"set", "count", "atomic"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			k := BlockKey{File: "foo.go", StartLine: 1, StartCol: 1, EndLine: 2, EndCol: 2, NumStmt: 5}
			p := &Profile{Mode: mode, Blocks: map[BlockKey]int{k: 3}}

			var buf bytes.Buffer
			require.NoError(t, WriteProfile(&buf, p))

			p2, err := ParseProfile(&buf)
			require.NoError(t, err)
			assert.Equal(t, mode, p2.Mode)
			require.Len(t, p2.Blocks, 1)
			if mode == "set" {
				assert.Equal(t, 1, p2.Blocks[k], "set 写出时规范化为 1")
			} else {
				assert.Equal(t, 3, p2.Blocks[k])
			}
		})
	}
}

// TestWriteProfile_SortedOrder 验证写出按 (file, startLine, startCol) 排序。
func TestWriteProfile_SortedOrder(t *testing.T) {
	t.Parallel()
	p := &Profile{
		Mode: "set",
		Blocks: map[BlockKey]int{
			{File: "z.go", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2, NumStmt: 1}: 1,
			{File: "a.go", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2, NumStmt: 1}: 1,
			{File: "m.go", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2, NumStmt: 1}: 1,
		},
	}
	var buf bytes.Buffer
	require.NoError(t, WriteProfile(&buf, p))
	out := buf.String()
	require.Greater(t, strings.Index(out, "a.go"), -1)
	require.Greater(t, strings.Index(out, "m.go"), strings.Index(out, "a.go"))
	require.Greater(t, strings.Index(out, "z.go"), strings.Index(out, "m.go"))
}

// TestEndToEnd_MergeFilesFromStrings 验证完整流程：解析两个 profile → 合并 → 写出 → 重读。
func TestEndToEnd_MergeFilesFromStrings(t *testing.T) {
	t.Parallel()
	in1 := profile("atomic",
		block("foo.go", 1, 1, 2, 2, 5, 3),
		block("foo.go", 3, 1, 4, 4, 2, 0),
	)
	in2 := profile("atomic",
		block("foo.go", 1, 1, 2, 2, 5, 7),
		block("foo.go", 5, 1, 6, 6, 1, 4),
	)

	p1, err := ParseProfile(strings.NewReader(in1))
	require.NoError(t, err)
	p2, err := ParseProfile(strings.NewReader(in2))
	require.NoError(t, err)

	merged, err := Merge([]*Profile{p1, p2})
	require.NoError(t, err)
	assert.Equal(t, "atomic", merged.Mode)

	var buf bytes.Buffer
	require.NoError(t, WriteProfile(&buf, merged))

	roundTrip, err := ParseProfile(&buf)
	require.NoError(t, err)
	assert.Equal(t, "atomic", roundTrip.Mode)

	k1 := BlockKey{File: "foo.go", StartLine: 1, StartCol: 1, EndLine: 2, EndCol: 2, NumStmt: 5}
	k2 := BlockKey{File: "foo.go", StartLine: 3, StartCol: 1, EndLine: 4, EndCol: 4, NumStmt: 2}
	k3 := BlockKey{File: "foo.go", StartLine: 5, StartCol: 1, EndLine: 6, EndCol: 6, NumStmt: 1}

	assert.Equal(t, 10, roundTrip.Blocks[k1], "atomic 合并应 SUM: 3+7")
	assert.Equal(t, 0, roundTrip.Blocks[k2], "仅 p1 出现的未命中块应保留为 0")
	assert.Equal(t, 4, roundTrip.Blocks[k3], "仅 p2 出现的命中块应保留")
}

// TestParseProfile_RepeatedBlock 验证同 profile 内重复块取最大值（幂等）。
func TestParseProfile_RepeatedBlock(t *testing.T) {
	t.Parallel()
	in := "mode: count\n" +
		block("foo.go", 1, 1, 2, 2, 5, 3) + "\n" +
		block("foo.go", 1, 1, 2, 2, 5, 8) + "\n"
	p, err := ParseProfile(strings.NewReader(in))
	require.NoError(t, err)
	require.Len(t, p.Blocks, 1)
	for _, c := range p.Blocks {
		assert.Equal(t, 8, c, "重复块应取最大值")
	}
}
