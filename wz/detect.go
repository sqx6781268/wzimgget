package wz

import "encoding/binary"

// validImgTags 为 img 根节点允许出现的类型标记名集合，
// 对应 WzComparerR2 的 Wz_Image.IsIllegalTag 判定。
var validImgTags = map[string]bool{
	"Property":         true,
	"Canvas":           true,
	"Shape2D#Vector2D": true,
	"Shape2D#Convex2D": true,
	"Sound_DX8":        true,
	"UOL":              true,
	"RawData":          true,
	"Canvas#Video":     true,
}

// DetectKeystream 参照 WzComparerR2 的 TryDetectEnc：
// 读取根节点类型标记字符串，依次用外部密钥（-key / 暴力枚举记录）与
// BMS/KMS/GMS 密钥流解密，能解出合法类型名的即为该文件使用的加密形式，
// 避免整文件重复试解析。
func DetectKeystream(data []byte) (*Keystream, string, bool) {
	for _, ks := range candidateKeystreams() {
		if tag, ok := readImgTagAt(data, ks, 0); ok && validImgTags[tag] {
			return ks, tag, true
		}
	}
	return nil, "", false
}

// readImgTagAt 在 off 处按 img 类型标记语法读取字符串（0x73 内联 / 0x1b 引用偏移），
// 任何越界都返回失败而不 panic。
func readImgTagAt(data []byte, ks *Keystream, off int) (string, bool) {
	if off >= len(data) {
		return "", false
	}
	switch data[off] {
	case 0x73:
		return readStringSafe(data, ks, off+1)
	case 0x1B:
		if off+5 > len(data) {
			return "", false
		}
		ref := int(int32(binary.LittleEndian.Uint32(data[off+1:])))
		return readStringSafe(data, ks, ref)
	}
	return "", false
}

// readStringSafe 在 off 处读取一个内联加密字符串（首字节为带符号长度：
// 负数=ANSI 长度，正数=UTF-16 字符数，±128/127 表示后跟 int32 长度），越界即失败。
func readStringSafe(data []byte, ks *Keystream, off int) (string, bool) {
	if off >= len(data) {
		return "", false
	}
	size := int(int8(data[off]))
	off++
	switch {
	case size < 0:
		if size == -128 {
			if off+4 > len(data) {
				return "", false
			}
			size = int(int32(binary.LittleEndian.Uint32(data[off:])))
			off += 4
		} else {
			size = -size
		}
		if size < 0 || off+size > len(data) {
			return "", false
		}
		b := append([]byte(nil), data[off:off+size]...)
		ks.Decrypt(b)
		mask := byte(0xAA)
		for i := range b {
			b[i] ^= mask
			mask++
		}
		return decodeAnsi(b), true
	case size > 0:
		if size == 127 {
			if off+4 > len(data) {
				return "", false
			}
			size = int(int32(binary.LittleEndian.Uint32(data[off:])))
			off += 4
		}
		if size < 0 || off+size*2 > len(data) {
			return "", false
		}
		b := append([]byte(nil), data[off:off+size*2]...)
		ks.Decrypt(b)
		mask := uint16(0xAAAA)
		runes := make([]rune, size)
		for i := 0; i < size; i++ {
			runes[i] = rune(binary.LittleEndian.Uint16(b[i*2:]) ^ mask)
			mask++
		}
		return string(runes), true
	}
	return "", true
}
