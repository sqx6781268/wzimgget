package wz

import (
	"crypto/aes"
	"encoding/binary"
	"runtime"
	"sync"
	"sync/atomic"
)

// 暴力枚举兜底（参照 Harepacker-resurrected WzKeyBruteforce 的两级判据思路，
// 针对独立 .img 调整判据：约束来自根类型标记字符串而非 .wz 目录图片名）：
//  1. 第一级：由文件开头根标记的密文反推密钥流首块若干字节的"期望值签名"，
//     对 0..2^32-1 全部 IV 生成密钥流首块做零分配快速比对；
//  2. 第二级：仅签名命中的候选，用完整密钥流解密根标记，
//     确认落在合法类型名集合内才算成功。

// bruteforceSig 表示"若 IV 正确，密钥流前若干字节应等于什么"的约束。
type bruteforceSig struct {
	need [16]byte // 期望的密钥流首块字节
	n    int      // 有效约束的字节数（只比较 need[:n]）
	tag  string   // 期望解出的根类型名
}

// rootCipherInfo 解析根标记，取出加密缓冲区的起始位置与长度。
func rootCipherInfo(data []byte) (cipherOff, nBytes int, utf16 bool, ok bool) {
	if len(data) < 2 {
		return 0, 0, false, false
	}
	strOff := 1
	switch data[0] {
	case 0x73: // 内联类型标记
	case 0x1B: // 引用：后跟 int32 绝对偏移
		if len(data) < 5 {
			return 0, 0, false, false
		}
		strOff = int(int32(binary.LittleEndian.Uint32(data[1:])))
	default:
		return 0, 0, false, false
	}
	if strOff <= 0 || strOff >= len(data) {
		return 0, 0, false, false
	}
	size := int(int8(data[strOff]))
	off := strOff + 1
	switch {
	case size < 0:
		if size == -128 { // 超长根标记，不做暴力枚举
			return 0, 0, false, false
		}
		return off, -size, false, true
	case size > 0:
		if size >= 127 { // 超长根标记，不做暴力枚举
			return 0, 0, false, false
		}
		return off, size * 2, true, true
	}
	return 0, 0, false, false
}

// buildBruteforceSigs 由根标记密文构造第一级签名集合。
func buildBruteforceSigs(data []byte) ([]bruteforceSig, bool) {
	off, nBytes, utf16, ok := rootCipherInfo(data)
	if !ok || nBytes <= 0 || off+nBytes > len(data) {
		return nil, false
	}
	if nBytes > 16 {
		nBytes = 16 // 第一级只约束密钥流首块
	}
	var sigs []bruteforceSig
	for tag := range validImgTags {
		s := bruteforceSig{tag: tag}
		for i := 0; i < nBytes && i <= len(tag); i++ {
			var plain byte
			if i < len(tag) {
				plain = tag[i]
			} // i == len(tag) 时期望值 0（字符串结束符）
			s.need[i] = data[off+i] ^ plain ^ bruteforceMask(utf16, i)
			s.n = i + 1
		}
		sigs = append(sigs, s)
	}
	return sigs, len(sigs) > 0
}

// bruteforceMask 返回解密掩码第 i 字节（ANSI 逐字节 0xAA+i；UTF-16 逐字符 0xAAAA+i 取对应半字节）。
func bruteforceMask(utf16 bool, i int) byte {
	if !utf16 {
		return byte(0xAA + i)
	}
	m := uint16(0xAAAA + i/2)
	if i%2 == 1 {
		return byte(m >> 8)
	}
	return byte(m)
}

// BruteforceIV 对给定 .img 数据全空间枚举 4 字节 IV，找回可用的密钥流。
// 命中后第二级验证必须真正解出合法根类型名。多核并行，无命中返回失败。
func BruteforceIV(data []byte) (*Keystream, string, bool) {
	sigs, ok := buildBruteforceSigs(data)
	if !ok {
		return nil, "", false
	}
	// 按签名首字节建 256 桶，热循环里 O(1) 过滤
	var bucket [256][]int
	for i := range sigs {
		bucket[sigs[i].need[0]] = append(bucket[sigs[i].need[0]], i)
	}
	cipher, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, "", false
	}
	type result struct {
		iv  [4]byte
		tag string
	}
	found := make(chan result, 1)
	var stopped atomic.Bool
	var wg sync.WaitGroup
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	const span = uint64(1) << 32
	for w := 0; w < workers; w++ {
		start := span * uint64(w) / uint64(workers)
		end := span * uint64(w+1) / uint64(workers)
		wg.Add(1)
		go func(start, end uint64) {
			defer wg.Done()
			var seed, keys [16]byte
			for ctr := start; ctr < end; ctr++ {
				if ctr&(1<<18-1) == 0 && stopped.Load() {
					return
				}
				iv := uint32(ctr)
				binary.LittleEndian.PutUint32(seed[0:], iv)
				binary.LittleEndian.PutUint32(seed[4:], iv)
				binary.LittleEndian.PutUint32(seed[8:], iv)
				binary.LittleEndian.PutUint32(seed[12:], iv)
				cipher.Encrypt(keys[:], seed[:])
				list := bucket[keys[0]]
				if len(list) == 0 {
					continue
				}
				// 第一级：完整签名比对
				for _, si := range list {
					s := &sigs[si]
					match := true
					for i := 1; i < s.n; i++ {
						if keys[i] != s.need[i] {
							match = false
							break
						}
					}
					if !match {
						continue
					}
					// 第二级：完整解密根标记验证
					ivb := [4]byte{byte(iv), byte(iv >> 8), byte(iv >> 16), byte(iv >> 24)}
					ks := NewKeystream(ivb)
					tag, ok := readImgTagAt(data, ks, 0)
					if ok && validImgTags[tag] {
						select {
						case found <- result{iv: ivb, tag: tag}:
							stopped.Store(true)
						default:
						}
						return
					}
				}
			}
		}(start, end)
	}
	allDone := make(chan struct{})
	go func() { wg.Wait(); close(allDone) }()
	select {
	case r := <-found:
		return NewKeystream(r.iv), r.tag, true
	case <-allDone:
		return nil, "", false
	}
}
