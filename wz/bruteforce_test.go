package wz

import (
	"encoding/binary"
	"testing"
	"time"
)

// makeRootTag 用指定 IV 构造一个以 "Property" 为根类型标记的独立 .img 文件头，
// 模拟"未知密钥"的样本，供暴力枚举测试。
func makeRootTag(iv [4]byte, tag string) []byte {
	n := len(tag)
	buf := make([]byte, n)
	copy(buf, tag)
	ks := NewKeystream(iv)
	stream := ks.Plain(n)
	for i := range buf {
		buf[i] ^= stream[i]      // 密钥流异或
		buf[i] ^= byte(0xAA + i) // ANSI 递增掩码
	}
	data := []byte{0x73, byte(-n)}
	return append(data, buf...)
}

func TestBruteforceIV(t *testing.T) {
	iv := [4]byte{0x78, 0x56, 0x34, 0x12} // uint32 LE = 0x12345678，位于枚举前段
	data := makeRootTag(iv, "Property")
	start := time.Now()
	ks, tag, ok := BruteforceIV(data)
	if !ok {
		t.Fatalf("暴力枚举未命中")
	}
	if ks.IV() != iv {
		t.Fatalf("IV 不符: %x", ks.IV())
	}
	if tag != "Property" {
		t.Fatalf("tag 不符: %s", tag)
	}
	t.Logf("命中耗时 %v", time.Since(start))
}

func TestBruteforceIV_HighRange(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过全空间长测")
	}
	var iv [4]byte
	binary.LittleEndian.PutUint32(iv[:], 0xFE000001) // 落在最后一个 worker 区间的靠后位置
	data := makeRootTag(iv, "Canvas")
	start := time.Now()
	ks, tag, ok := BruteforceIV(data)
	if !ok {
		t.Fatalf("暴力枚举未命中")
	}
	if ks.IV() != iv || tag != "Canvas" {
		t.Fatalf("结果不符: IV=%x tag=%s", ks.IV(), tag)
	}
	t.Logf("命中耗时 %v", time.Since(start))
}

func TestBruteforceIV_NoHit(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过全空间长测")
	}
	data := []byte{0x73, 0xF8, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	// 该密文几乎不可能解出合法类型名，应完整扫完并失败
	if _, _, ok := BruteforceIV(data); ok {
		t.Fatalf("不应命中")
	}
}
