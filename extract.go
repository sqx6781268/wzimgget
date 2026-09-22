package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wzimgget/wz"
)

// 只提取这些顶层目录下的图标：装备（Character）、物品（Item）、NPC（Npc）。
var targetDirs = map[string]bool{
	"character": true,
	"item":      true,
	"npc":       true,
}

// iconCandidates 为图标节点候选路径，顺序参照 WzComparerR2 的图标取值逻辑：
// 优先 info 下的 icon/iconRaw，其次是各动作的首帧画布。
var iconCandidates = []string{
	"info/iconRaw",
	"info/icon",
	"info/animatedIcon",
	"stand0/0",
	"stand/0",
	"strike1/0",
	"swingO1/0",
	"action/00",
	"action/stick0",
	"0",
}

type stats struct {
	total, ok, skipped, failed int
}

// splitArgs 手动分离位置参数与选项：
// 标准库 flag 在遇到首个非选项参数后即停止解析，
// 这里预处理使 `extract <data> -out X -v` 等任意顺序均可。
func splitArgs(args []string) (flagArgs, posArgs []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-v" || a == "--v" || strings.HasPrefix(a, "-v=") || strings.HasPrefix(a, "--v="):
			flagArgs = append(flagArgs, a)
		case (a == "-nobf" || a == "--nobf") && !strings.Contains(a, "="):
			flagArgs = append(flagArgs, a)
		case (a == "-out" || a == "--out" || a == "-key" || a == "--key") && i+1 < len(args):
			flagArgs = append(flagArgs, a, args[i+1])
			i++
		case strings.HasPrefix(a, "-out=") || strings.HasPrefix(a, "--out=") ||
			strings.HasPrefix(a, "-key=") || strings.HasPrefix(a, "--key="):
			flagArgs = append(flagArgs, a)
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, "未知选项: %s\n", a)
			os.Exit(1)
		default:
			posArgs = append(posArgs, a)
		}
	}
	return
}

func cmdExtract(args []string) {
	flagArgs, posArgs := splitArgs(args)

	fs := flag.NewFlagSet("extract", flag.ExitOnError)
	out := fs.String("out", "", "输出目录，默认为 data 同级目录下的 imgdata")
	verbose := fs.Bool("v", false, "输出每个文件的处理结果")
	key := fs.String("key", "", "外部密钥，逗号分隔：8位十六进制IV 或 64位十六进制32字节用户密钥（优先于内置密钥尝试）")
	nobf := fs.Bool("nobf", false, "禁用密钥全部失败时的 IV 暴力枚举兜底")
	fs.Parse(flagArgs)

	if *key != "" {
		if err := setupKeys(*key); err != nil {
			fmt.Fprintln(os.Stderr, "错误:", err)
			os.Exit(1)
		}
	}

	dataDir := ""
	if len(posArgs) > 0 {
		dataDir = posArgs[0]
	}
	if dataDir == "" {
		fmt.Fprintln(os.Stderr, "错误: 需要传入 data 目录路径，例如: wzimgget extract D:\\game\\Data")
		os.Exit(1)
	}
	inAbs, err := filepath.Abs(dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
	if st, err := os.Stat(inAbs); err != nil || !st.IsDir() {
		fmt.Fprintf(os.Stderr, "错误: data 目录不存在: %s\n", inAbs)
		os.Exit(1)
	}
	outDir := *out
	if outDir == "" {
		// 默认输出到 data 的同级目录下的 imgdata
		outDir = filepath.Join(filepath.Dir(inAbs), "imgdata")
	}
	outAbs, _ := filepath.Abs(outDir)
	keyLogPath := filepath.Join(filepath.Dir(inAbs), "wzimgget_keys.log")
	fmt.Printf("data 目录: %s\n输出目录: %s\n", inAbs, outAbs)

	var st stats
	err = filepath.Walk(inAbs, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "警告: %v\n", err)
			return nil
		}
		rel, _ := filepath.Rel(inAbs, path)
		relSlash := filepath.ToSlash(rel)
		if info.IsDir() {
			// 顶层目录过滤：只进入 Character/Item/Npc
			if relSlash != "." {
				top := strings.ToLower(strings.SplitN(relSlash, "/", 2)[0])
				if !targetDirs[top] {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".img") {
			return nil
		}
		st.total++
		res, err := extractOne(path, inAbs, outAbs, !*nobf, keyLogPath)
		if err != nil {
			st.failed++
			fmt.Fprintf(os.Stderr, "失败 %s: %v\n", relSlash, err)
			return nil
		}
		if res == "" {
			st.skipped++
			if *verbose {
				fmt.Printf("跳过 %s: 未找到图标\n", relSlash)
			}
			return nil
		}
		st.ok++
		if *verbose {
			fmt.Printf("完成 %s -> %s\n", relSlash, res)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "遍历失败:", err)
		os.Exit(1)
	}
	fmt.Printf("共 %d 个 img：成功 %d，无图标 %d，失败 %d\n", st.total, st.ok, st.skipped, st.failed)
}

// loadImgFile 读取并解析单个 img 文件（含暴力枚举兜底）。
func loadImgFile(path string, allowBF bool, keyLogPath string) (*wz.Img, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return loadWithBruteforce(data, filepath.Base(path), allowBF, keyLogPath)
}

// pickIcon 按候选顺序在属性树中查找第一个可解码的图标画布，返回 PNG 数据；
// 返回 (nil, nil) 表示树中没有任何图标候选节点。
func pickIcon(img *wz.Img) ([]byte, error) {
	root := img.Root
	var decodeErr error
	for _, cand := range iconCandidates {
		n := root.Resolve(cand)
		if n == nil {
			continue
		}
		n = root.FollowUOL(n)
		if n == nil || n.Kind != wz.KindCanvas {
			continue
		}
		pngData, err := img.DecodePNG(n)
		if err != nil {
			// 该候选画布数据异常（如空帧），继续尝试下一个候选
			decodeErr = err
			continue
		}
		return pngData, nil
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("解码图标失败: %w", decodeErr)
	}
	return nil, nil
}

// extractOne 提取单个 img 的主图标，输出为 <out>/<相对路径>/<name>.img.png。
func extractOne(path, inAbs, outAbs string, allowBF bool, keyLogPath string) (string, error) {
	img, err := loadImgFile(path, allowBF, keyLogPath)
	if err != nil {
		return "", err
	}
	pngData, err := pickIcon(img)
	if err != nil {
		return "", err
	}
	if pngData == nil {
		return "", nil
	}
	rel, _ := filepath.Rel(inAbs, path)
	dst := filepath.Join(outAbs, filepath.FromSlash(rel)+".png")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(dst, pngData, 0o644); err != nil {
		return "", err
	}
	return filepath.ToSlash(dst), nil
}
