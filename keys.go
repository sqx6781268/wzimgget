package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wzimgget/wz"
)

// setupKeys 解析 -key 参数值（逗号分隔多个）。
// 8 位十六进制 = 自定义 4 字节 IV；64 位十六进制 = 自定义 32 字节 AES 用户密钥
// （与内置 KMS/GMS IV 组合注册，用于 ZLZ 动态密钥等场景）。外部密钥优先于内置密钥尝试。
func setupKeys(spec string) error {
	for _, item := range strings.Split(spec, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		b, err := hex.DecodeString(item)
		if err != nil {
			return fmt.Errorf("-key 值 %q 不是合法十六进制: %v", item, err)
		}
		switch len(b) {
		case 4:
			iv := [4]byte{b[0], b[1], b[2], b[3]}
			wz.AddExternalKeystream(wz.NewKeystream(iv))
			fmt.Printf("已注册外部密钥 IV=%s\n", strings.ToUpper(item))
		case 32:
			for _, iv := range [][4]byte{wz.IV_KMS, wz.IV_GMS} {
				wz.AddExternalKeystream(wz.NewKeystreamWithUserKey(b, iv))
			}
			fmt.Printf("已注册外部 32 字节用户密钥（配合内置 IV 尝试）\n")
		default:
			return fmt.Errorf("-key 值 %q 长度 %d 字节，应为 4 字节 IV（8 位十六进制）或 32 字节用户密钥（64 位十六进制）", item, len(b))
		}
	}
	return nil
}

// loadWithBruteforce 解析 img 数据：常规密钥全部失败且允许暴力枚举时，
// 按文档《WZ密钥与版本对照及暴力枚举兜底说明》做全 2^32 IV 空间枚举；
// 命中后记录密钥（后续提取优先尝试该密钥）并写日志，未命中则返回原始错误。
func loadWithBruteforce(data []byte, srcName string, allowBF bool, logPath string) (*wz.Img, error) {
	img, err := wz.Load(data)
	if err == nil || !allowBF {
		return img, err
	}
	fmt.Printf("已知密钥均无法解密 %s，启动 IV 暴力枚举（0..2^32-1）...\n", srcName)
	start := time.Now()
	ks, tag, ok := wz.BruteforceIV(data)
	if !ok {
		fmt.Fprintf(os.Stderr, "暴力枚举失败 %s：未找到可用 IV（耗时 %v）\n", srcName, time.Since(start).Round(time.Millisecond))
		return img, err
	}
	iv := ks.IV()
	line := fmt.Sprintf("%s\t暴力枚举命中\tfile=%s\tIV=%02X%02X%02X%02X\ttag=%s\t耗时=%s",
		time.Now().Format("2006-01-02 15:04:05"), srcName, iv[0], iv[1], iv[2], iv[3], tag, time.Since(start).Round(time.Millisecond))
	fmt.Println(line)
	fmt.Println("已记录该密钥，本次运行后续文件将优先使用它解密")
	appendKeyLog(logPath, line)
	wz.AddExternalKeystream(ks)
	return wz.Load(data)
}

// appendKeyLog 将密钥记录追加写入日志文件（路径为空则跳过）。
func appendKeyLog(path, line string) {
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "密钥日志目录创建失败:", err)
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "密钥日志写入失败:", err)
		return
	}
	defer f.Close()
	fmt.Fprintln(f, line)
}
