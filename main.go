package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wzimgget/wz"
)

// 命令行入口：
//
//	wzimgget extract <Data目录> [-out 输出目录] [-v]  批量提取图标（默认输出到 Data 同级 imgdata）
//	wzimgget dump <xx.img>                            打印 img 属性树（调试用）
//	wzimgget png <xx.img> <节点路径> <out.png>         导出指定 Canvas 节点为 PNG（调试用）
func main() {
	if len(os.Args) < 2 {
		// 无参数直接运行：若二进制同级目录存在 Data 目录，则自动提取图标
		if dir, ok := siblingDataDir(); ok {
			fmt.Printf("未指定参数，检测到同级 Data 目录：%s\n开始提取图标...\n", dir)
			cmdExtract([]string{dir})
			pause()
			return
		}
		usage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "dump":
		if len(os.Args) < 3 {
			usage()
			os.Exit(1)
		}
		cmdDump(os.Args[2])
	case "png":
		if len(os.Args) < 5 {
			usage()
			os.Exit(1)
		}
		cmdPng(os.Args[2], os.Args[3], os.Args[4])
	case "extract":
		cmdExtract(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

// siblingDataDir 在二进制所在目录及当前工作目录中查找名为 Data 的子目录（忽略大小写）。
func siblingDataDir() (string, bool) {
	var bases []string
	if exe, err := os.Executable(); err == nil {
		bases = append(bases, filepath.Dir(exe))
	}
	if cwd, err := os.Getwd(); err == nil {
		bases = append(bases, cwd)
	}
	for _, base := range bases {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() && strings.EqualFold(e.Name(), "Data") {
				return filepath.Join(base, e.Name()), true
			}
		}
	}
	return "", false
}

// pause 在无参数（双击）运行结束时等待回车，避免控制台窗口瞬间关闭。
func pause() {
	fmt.Print("按回车键退出...")
	fmt.Scanln()
}

func usage() {
	fmt.Fprintln(os.Stderr, "用法: wzimgget extract <Data目录> [-out 输出目录] [-v] | dump <file.img> | png <file.img> <path> <out.png>")
	fmt.Fprintln(os.Stderr, "提示: 将本程序放在 Data 目录同级时直接双击运行，可自动提取 Data 下的图标到同级 imgdata")
}

func loadFile(path string) (*wz.Img, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	img, err := wz.Load(data)
	if err != nil {
		return img, fmt.Errorf("%s: %w", path, err)
	}
	return img, nil
}

func cmdDump(path string) {
	img, err := loadFile(path)
	if img != nil {
		dumpNode(img.Root, 0)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func dumpNode(n *wz.Node, depth int) {
	indent := ""
	for i := 0; i < depth; i++ {
		indent += "  "
	}
	info := ""
	switch n.Kind {
	case wz.KindInt:
		info = fmt.Sprintf(" = %d", n.I)
	case wz.KindFloat, wz.KindDouble:
		info = fmt.Sprintf(" = %g", n.F)
	case wz.KindString:
		info = fmt.Sprintf(" = %q", n.S)
	case wz.KindVector:
		info = fmt.Sprintf(" = (%d,%d)", n.X, n.Y)
	case wz.KindCanvas:
		p := n.Png
		info = fmt.Sprintf(" [%dx%d form=%d off=%d len=%d]", p.W, p.H, p.Form, p.Off, p.Length)
	case wz.KindRaw, wz.KindSound:
		info = fmt.Sprintf(" [off=%d len=%d]", n.RawOff, n.RawLen)
	}
	fmt.Printf("%s%s (%s) @%d%s\n", indent, n.Name, n.Kind, n.AtOff, info)
	for _, c := range n.Children {
		dumpNode(c, depth+1)
	}
}

func cmdPng(path, nodePath, out string) {
	img, err := loadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
	root := img.Root
	n := root.Resolve(nodePath)
	if n == nil {
		n = root.FollowUOL(root.Resolve(nodePath))
	}
	if n == nil {
		fmt.Fprintln(os.Stderr, "找不到节点:", nodePath)
		os.Exit(1)
	}
	if n.Kind == wz.KindUOL {
		n = root.FollowUOL(n)
	}
	data, err := img.DecodePNG(n)
	if err != nil {
		fmt.Fprintln(os.Stderr, "解码失败:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "写入失败:", err)
		os.Exit(1)
	}
	fmt.Printf("已导出 %s (%d 字节)\n", out, len(data))
}
