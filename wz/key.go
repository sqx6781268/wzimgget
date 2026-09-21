// Package wz 解析 MapleStory WZ 独立 .img 文件（参照 WzComparerR2 / MapleLib 的解析思路）。
package wz

import (
	"crypto/aes"
)

// aesKey 为 WZ 字符串/数据解密使用的固定 AES-256-ECB 密钥（来自官方客户端）。
var aesKey = []byte{
	0x13, 0x00, 0x00, 0x00,
	0x08, 0x00, 0x00, 0x00,
	0x06, 0x00, 0x00, 0x00,
	0xB4, 0x00, 0x00, 0x00,
	0x1B, 0x00, 0x00, 0x00,
	0x0F, 0x00, 0x00, 0x00,
	0x33, 0x00, 0x00, 0x00,
	0x52, 0x00, 0x00, 0x00,
}

// 各地区版本的 IV
var (
	IV_KMS = [4]byte{0xb9, 0x7d, 0x63, 0xe9}
	IV_GMS = [4]byte{0x4d, 0x23, 0xc7, 0x2b}
	IV_BMS = [4]byte{0x00, 0x00, 0x00, 0x00}
)

// Keystream 由 IV 经 AES-ECB 链式加密生成的密钥流，用于解密字符串与部分数据块。
// 与 WzComparerR2 一致：每个缓冲区都从密钥流下标 0 开始异或。
type Keystream struct {
	iv   [4]byte
	keys []byte
	zero bool // BMS 空 IV：不加密
}

func NewKeystream(iv [4]byte) *Keystream {
	k := &Keystream{iv: iv}
	if iv == [4]byte{} {
		k.zero = true
	}
	return k
}

func (k *Keystream) ensure(n int) {
	if k.zero || len(k.keys) >= n {
		return
	}
	blockSize := 16
	blocks := (n + blockSize - 1) / blockSize
	newKeys := make([]byte, blocks*blockSize)
	cipher, err := aes.NewCipher(aesKey)
	if err != nil {
		panic(err)
	}
	start := len(k.keys)
	// 扩容时必须把已生成的旧密钥流原样复制到新数组前缀，
	// 否则链式加密会以零块作为前驱，导致该位置之后的所有密钥全部错误。
	if start > 0 {
		copy(newKeys[:start], k.keys)
	}
	for i := start; i < len(newKeys); i += blockSize {
		if i == 0 {
			block := make([]byte, blockSize)
			for j := 0; j < blockSize; j++ {
				block[j] = k.iv[j%4]
			}
			cipher.Encrypt(newKeys[i:i+blockSize], block)
		} else {
			cipher.Encrypt(newKeys[i:i+blockSize], newKeys[i-blockSize:i])
		}
	}
	k.keys = newKeys
}

// Decrypt 从密钥流下标 0 开始原地异或解密缓冲区。
func (k *Keystream) Decrypt(buf []byte) {
	if k.zero || len(buf) == 0 {
		return
	}
	k.ensure(len(buf))
	for i := range buf {
		buf[i] ^= k.keys[i]
	}
}

// DecryptAt 从密钥流下标 off 开始原地异或解密（用于 Canvas 的旧式加密数据块，按缓冲区起点即 off=0 使用，与 WzComparerR2 保持兼容行为）。
func (k *Keystream) DecryptAt(buf []byte, off int) {
	if k.zero || len(buf) == 0 {
		return
	}
	k.ensure(off + len(buf))
	for i := range buf {
		buf[i] ^= k.keys[off+i]
	}
}

// Plain 返回密钥流的原始字节（调试用）。
func (k *Keystream) Plain(n int) []byte {
	k.ensure(n)
	if k.zero {
		return make([]byte, n)
	}
	return k.keys[:n]
}
