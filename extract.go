package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wzimgget/wz"
)

// 只提取这些顶层目录下的图标：装备（Character）、物品（Item）、NPC（Npc）、
// 怪物（Mob）、技能（Skill）、变身（Morph）、地图物件（Reactor）。
var targetDirs = map[string]bool{
	"character": true,
	"item":      true,
	"npc":       true,
	"mob":       true,
	"skill":     true,
	"morph":     true,
	"reactor":   true,
}

// iconCandidates 为图标节点候选路径，顺序参照 WzComparerR2 的图标取值逻辑：
// 优先 info 下的 icon/iconRaw；其次是 Special 组文件的直接 icon 或 iconRaw/N 动画首帧；
// 最后是各动作的首帧画布（NPC/怪物等非物品资源）。
var iconCandidates = []string{
	"info/iconRaw",
	"info/icon",
	"info/animatedIcon",
	"iconRaw",
	"icon",
	"iconRaw/0",
	"icon/0",
	"stand0/0",
	"stand/0",
	"strike1/0",
	"swingO1/0",
	"action/00",
	"action/stick0",
	"0/0",
	"0",
}

// hairFaceCandidates 为 -hairface 开启时追加的穿戴部件代表图候选。
var hairFaceCandidates = []string{
	"default/hairOverHead",
	"default/face",
}

// optHairFace 由命令行 -hairface 设置：是否导出 Hair/Face 部件代表图。
var optHairFace bool

// activeCandidates 返回当前生效的候选路径列表。
func activeCandidates() []string {
	if optHairFace {
		return append(iconCandidates[:len(iconCandidates):len(iconCandidates)], hairFaceCandidates...)
	}
	return iconCandidates
}

type stats struct {
	total, ok, skipped, failed, icons int
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
		case (a == "-nobf" || a == "--nobf" || a == "-nozip" || a == "--nozip" ||
			a == "-hairface" || a == "--hairface") && !strings.Contains(a, "="):
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
	nozip := fs.Bool("nozip", false, "提取完成后不自动打包 imgdata.zip（Store 模式，便于备份传输与零解压读取）")
	fs.BoolVar(&optHairFace, "hairface", false, "同时导出 Hair/Face 穿戴部件代表图（default/hairOverHead、default/face）")
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
		top := strings.ToLower(strings.SplitN(relSlash, "/", 2)[0])
		outs, err := extractOne(path, inAbs, outAbs, top == "item", top == "morph", !*nobf, keyLogPath)
		if err != nil {
			st.failed++
			fmt.Fprintf(os.Stderr, "失败 %s: %v\n", relSlash, err)
			return nil
		}
		if len(outs) == 0 {
			st.skipped++
			if *verbose {
				fmt.Printf("跳过 %s: 未找到图标\n", relSlash)
			}
			return nil
		}
		st.ok++
		st.icons += len(outs)
		if *verbose {
			if len(outs) == 1 {
				fmt.Printf("完成 %s -> %s\n", relSlash, outs[0])
			} else {
				fmt.Printf("完成 %s -> %d 个物品图标\n", relSlash, len(outs))
			}
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "遍历失败:", err)
		os.Exit(1)
	}
	fmt.Printf("共 %d 个 img：成功 %d（含 %d 张图标），无图标 %d，失败 %d\n", st.total, st.ok, st.icons, st.skipped, st.failed)
	if !*nozip {
		zipPath := outAbs + ".zip"
		count, total, err := packStore(outAbs, zipPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "打包失败:", err)
			return
		}
		fmt.Printf("已打包 %s（%d 个文件，原始 %s，Store 模式）\n", zipPath, count, humanSize(total))
		fmt.Println("提示: 备份或校验后可删除源目录；目录内容再有增删时必须重新打包（wzimgget pack 输出目录）")
	}
}

// loadImgFile 读取并解析单个 img 文件（含暴力枚举兜底）。
func loadImgFile(path string, allowBF bool, keyLogPath string) (*wz.Img, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return loadWithBruteforce(data, filepath.Base(path), allowBF, keyLogPath)
}

// hasSegment 判断路径中是否含指定目录段（单文件模式下据此启用组文件/深扫模式）。
func hasSegment(path, seg string) bool {
	for _, s := range strings.Split(filepath.ToSlash(path), "/") {
		if strings.EqualFold(s, seg) {
			return true
		}
	}
	return false
}

// pickFrom 在 node 子树下按候选路径查找第一个可解码的图标画布；
// 返回 (nil, nil) 表示无候选节点，返回 (nil, err) 表示候选存在但全部解码失败。
func pickFrom(img *wz.Img, node *wz.Node, cands []string) ([]byte, error) {
	var decodeErr error
	for _, cand := range cands {
		n := node.Resolve(cand)
		if n == nil {
			continue
		}
		n = img.Root.FollowUOL(n)
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
	return nil, decodeErr
}

// extractOne 提取单个 img 的图标：
// 根级有图标（装备/单件物品/NPC/怪物/技能）输出一张 <相对路径>.img.png；
// group（Item 组文件）为每个子节点（物品 ID）各输出一张 <物品ID>.img.png；
// deep（Morph 等：图标在 <动作>/帧）下探一层只取一张。
func extractOne(path, inAbs, outAbs string, group, deep, allowBF bool, keyLogPath string) ([]string, error) {
	img, err := loadImgFile(path, allowBF, keyLogPath)
	if err != nil {
		return nil, err
	}
	rel, _ := filepath.Rel(inAbs, path)
	data, err := pickFrom(img, img.Root, activeCandidates())
	if err != nil {
		return nil, fmt.Errorf("解码图标失败: %w", err)
	}
	if data == nil && deep {
		data, err = pickNested(img)
		if err != nil {
			return nil, fmt.Errorf("解码图标失败: %w", err)
		}
	}
	if data != nil {
		dst := filepath.Join(outAbs, filepath.FromSlash(rel)+".png")
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return nil, err
		}
		return []string{filepath.ToSlash(dst)}, nil
	}
	if group {
		return extractGroup(img, rel, outAbs)
	}
	return nil, nil
}

// pickNested 下探一层子节点，返回第一个可解码的图标（一图一对象的域用，如 Morph 的 fly/0）。
func pickNested(img *wz.Img) ([]byte, error) {
	var decodeErr error
	for _, child := range img.Root.Children {
		data, err := pickFrom(img, child, activeCandidates())
		if data != nil {
			return data, nil
		}
		if err != nil {
			decodeErr = err
		}
	}
	return nil, decodeErr
}

// extractGroup 处理 Item 组文件：逐子节点（物品 ID）提取图标，
// 输出到组文件所在输出目录下的 <物品ID>.img.png。
func extractGroup(img *wz.Img, rel, outAbs string) ([]string, error) {
	dir := filepath.Join(outAbs, filepath.FromSlash(filepath.Dir(rel)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var outs []string
	var lastErr error
	for _, child := range img.Root.Children {
		data, err := pickFrom(img, child, activeCandidates())
		if err != nil {
			lastErr = err
			continue
		}
		if data == nil {
			continue
		}
		dst := filepath.Join(dir, sanitizeName(child.Name)+".img.png")
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return outs, err
		}
		outs = append(outs, filepath.ToSlash(dst))
	}
	if len(outs) == 0 && lastErr != nil {
		return nil, fmt.Errorf("解码图标失败: %w", lastErr)
	}
	return outs, nil
}

// sanitizeName 替换 Windows 文件名非法字符，防止异常节点名写出意外路径。
func sanitizeName(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|', 0:
			return '_'
		}
		return r
	}, s)
}
