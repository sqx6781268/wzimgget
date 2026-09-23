package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// packStore 将 srcDir 目录下全部文件以 Store 模式（不压缩）打包为 ZIP，
// 归档内相对路径与原目录结构一致。PNG 本身已压缩，再 Deflate 收益极低，
// Store 模式可实现读取时零解压，同时消除海量小文件在 NTFS 上的簇空间浪费。
// 返回打包文件数与原始数据总字节数。
func packStore(srcDir, zipPath string) (int, int64, error) {
	f, err := os.Create(zipPath)
	if err != nil {
		return 0, 0, err
	}
	w := zip.NewWriter(f)
	var count int
	var total int64
	zipSlash := filepath.ToSlash(zipPath)
	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// 输出目录若嵌套在源目录内，跳过 ZIP 自身
		if filepath.ToSlash(path) == zipSlash {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel) // ZIP 规范使用 / 分隔符
		h.Method = zip.Store
		cw, err := w.CreateHeader(h)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		n, err := io.Copy(cw, in)
		if err != nil {
			return err
		}
		count++
		total += n
		return nil
	})
	if err == nil {
		if err = w.Close(); err == nil {
			err = f.Close()
		}
	}
	if err != nil {
		f.Close()
		os.Remove(zipPath) // 失败时清理半成品
		return 0, 0, err
	}
	return count, total, nil
}

// cmdPack 独立打包命令：wzimgget pack <目录> [-out x.zip]
func cmdPack(args []string) {
	flagArgs, posArgs := splitArgs(args)
	fs := flag.NewFlagSet("pack", flag.ExitOnError)
	out := fs.String("out", "", "输出 ZIP 路径，默认为 <目录>.zip（目录同级）")
	fs.Parse(flagArgs)
	if len(posArgs) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 需要传入待打包目录，例如: wzimgget pack D:\\game\\imgdata")
		os.Exit(1)
	}
	srcAbs, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
	if st, err := os.Stat(srcAbs); err != nil || !st.IsDir() {
		fmt.Fprintf(os.Stderr, "错误: 目录不存在: %s\n", srcAbs)
		os.Exit(1)
	}
	zipPath := *out
	if zipPath == "" {
		zipPath = srcAbs + ".zip"
	}
	zipAbs, _ := filepath.Abs(zipPath)
	start := time.Now()
	count, total, err := packStore(srcAbs, zipAbs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "打包失败:", err)
		os.Exit(1)
	}
	st, _ := os.Stat(zipAbs)
	fmt.Printf("打包完成: %s（%d 个文件，原始 %s，ZIP %s，Store 模式，耗时 %v）\n",
		zipAbs, count, humanSize(total), humanSize(st.Size()), time.Since(start).Round(time.Millisecond))
	fmt.Println("提示: 备份或校验后可删除源目录；目录内容再有增删时必须重新打包")
}

// humanSize 将字节数格式化为易读单位。
func humanSize(b int64) string {
	const m = 1024 * 1024
	if b >= m {
		return fmt.Sprintf("%.1f MB", float64(b)/m)
	}
	return fmt.Sprintf("%.1f KB", float64(b)/1024)
}
