//go:build ignore

// Package main 合并多个 Go 覆盖率 profile（支持 set/count/atomic）。
//
// `go tool cover` 没有内置 merge 子命令，本脚本调用 scripts/mergecoverage 包完成实际工作。
// 之所以保留 //go:build ignore 的 main 包装，是为了让 go run scripts/merge_coverage.go ...
// 这种用法不被 go build ./... / go test ./... / go vet ./... 编译，
// 而真正的合并逻辑放在 scripts/mergecoverage 包内被正常测试覆盖。
//
// Usage:
//
//	go run scripts/merge_coverage.go -o <output> <input1> <input2> [inputN...]
//
// 输入 profile 模式必须一致（set/count/atomic），输出保留该模式。
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/example/sso/scripts/mergecoverage"
)

func main() {
	output := flag.String("o", "", "output profile path (required)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: go run scripts/merge_coverage.go -o <output> <input1> <input2> [inputN...]\n")
		fmt.Fprintf(os.Stderr, "Merges coverage profiles (set/count/atomic) into a single profile.\n")
		fmt.Fprintf(os.Stderr, "All inputs must share the same mode; the output preserves it.\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	inputs := flag.Args()
	if *output == "" || len(inputs) < 2 {
		flag.Usage()
		os.Exit(2)
	}

	profiles := make([]*mergecoverage.Profile, 0, len(inputs))
	for _, in := range inputs {
		p, err := mergecoverage.ParseProfileFile(in)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		profiles = append(profiles, p)
	}

	merged, err := mergecoverage.Merge(profiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := mergecoverage.WriteProfileFile(*output, merged); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
