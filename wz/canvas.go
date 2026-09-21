package wz

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
)

// DecodePNG 将 Canvas 节点解码为 PNG 字节。
// 参照 WzComparerR2：数据可能为原始 PNG、zlib 压缩的像素数据（多种像素格式）、
// 或旧式 AES 分块加密后的 deflate 数据。
func (img *Img) DecodePNG(n *Node) ([]byte, error) {
	if n.Kind != KindCanvas || n.Png == nil {
		return nil, fmt.Errorf("节点 %s 不是 Canvas", n.Name)
	}
	pi := n.Png
	if pi.Off < 0 || pi.Off+pi.Length > len(img.Data) {
		return nil, fmt.Errorf("PNG 数据越界: off=%d len=%d file=%d", pi.Off, pi.Length, len(img.Data))
	}
	raw := img.Data[pi.Off : pi.Off+pi.Length]

	// 原始 PNG 数据直接返回
	if len(raw) > 4 && bytes.Equal(raw[:4], []byte{0x89, 'P', 'N', 'G'}) {
		return append([]byte(nil), raw...), nil
	}
	// 原始 BMP 数据
	if len(raw) > 2 && raw[0] == 'B' && raw[1] == 'M' {
		return bmpToPNG(raw)
	}

	var pixel []byte
	var err error
	if pi.Form == 0 {
		// 未压缩的原始像素数据（ARGB8888）
		pixel = raw
	} else {
		var comp []byte
		comp, err = decompressCanvas(img, raw)
		if err != nil {
			return nil, err
		}
		pixel, err = img.decodeForm(pi, comp)
		if err != nil {
			return nil, err
		}
	}
	return encodeARGB(pi.W, pi.H, pixel)
}

// isZlibHeader 判断缓冲区是否为 zlib 流头部（CMF=0x78 且满足 FLG 校验），
// 兼容 78 9c / 78 da / 78 01 等不同压缩级别，区别于旧式 AES 加密块数据。
func isZlibHeader(b []byte) bool {
	if len(b) < 2 || b[0] != 0x78 {
		return false
	}
	return (uint16(b[0])<<8|uint16(b[1]))%31 == 0
}

// decompressCanvas 处理 zlib 头 / 旧式加密分块两种情况，返回解压后的像素数据。
func decompressCanvas(img *Img, raw []byte) ([]byte, error) {
	if len(raw) < 4 {
		return nil, fmt.Errorf("数据过短")
	}
	var payload []byte
	if isZlibHeader(raw) {
		// 未压缩标志位场景：数据即 zlib 流（78 9c / 78 da / 78 01 等），直接取内容
		payload = raw
	} else {
		// 旧式：int32 长度 + AES 密钥流解密的数据块序列
		var buf bytes.Buffer
		r := bytes.NewReader(raw)
		var hdr [4]byte
		for r.Len() > 0 {
			if _, err := r.Read(hdr[:]); err != nil {
				return nil, err
			}
			l := int(binary.LittleEndian.Uint32(hdr[:]))
			if l < 0 || l > r.Len() {
				return nil, fmt.Errorf("非法数据块长度 %d", l)
			}
			blk := make([]byte, l)
			r.Read(blk)
			img.Ks.Decrypt(blk)
			buf.Write(blk)
		}
		payload = buf.Bytes()
	}
	// payload 前 2 字节为 zlib 头，跳过
	fr := flate.NewReader(bytes.NewReader(payload[2:]))
	defer fr.Close()
	var out bytes.Buffer
	if _, err := out.ReadFrom(fr); err != nil && out.Len() == 0 {
		return nil, fmt.Errorf("解压失败: %w", err)
	}
	return out.Bytes(), nil
}

// decodeForm 将解压后的像素数据转换为 BGRA 字节序列（与 WzComparerR2 表一致）。
func (img *Img) decodeForm(pi *PngInfo, data []byte) ([]byte, error) {
	w, h := pi.W, pi.H
	switch pi.Form {
	case 1: // 16 位 ARGB4444
		if len(data) < w*h*2 {
			return nil, fmt.Errorf("form1 数据不足")
		}
		out := make([]byte, w*h*4)
		for i := 0; i < w*h*2; i++ {
			lo := data[i] & 0x0F
			hi := data[i] >> 4
			out[i*2] = lo | lo<<4
			out[i*2+1] = hi | hi<<4
		}
		return out, nil
	case 2, 0: // 32 位 ARGB8888
		if len(data) < w*h*4 {
			return nil, fmt.Errorf("form2 数据不足: %d < %d", len(data), w*h*4)
		}
		return data[:w*h*4], nil
	case 3: // 4x4 块缩放位图
		bw := ((w + 3) / 4) * 4
		bh := ((h + 3) / 4) * 4
		if len(data) < bw*bh/2 {
			return nil, fmt.Errorf("form3 数据不足")
		}
		out := make([]byte, w*h*4)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				sx, sy := x/4, y/4
				idx := (sy*bw/4 + sx) * 2
				b0, b1 := data[idx], data[idx+1]
				lo0, hi0 := b0&0x0F|b0&0x0F<<4, b0>>4|b0>>4<<4
				lo1, hi1 := b1&0x0F|b1&0x0F<<4, b1>>4|b1>>4<<4
				p := (y*w + x) * 4
				out[p], out[p+1], out[p+2], out[p+3] = lo0, hi0, lo1, hi1
			}
		}
		return out, nil
	case 513: // RGB565
		if len(data) < w*h*2 {
			return nil, fmt.Errorf("form513 数据不足")
		}
		out := make([]byte, w*h*4)
		for i := 0; i < w*h; i++ {
			v := binary.LittleEndian.Uint16(data[i*2:])
			r5, g6, b5 := v>>11, (v>>5)&0x3F, v&0x1F
			r := (r5<<3 | r5>>2) & 0xFF
			g := (g6<<2 | g6>>4) & 0xFF
			b := (b5<<3 | b5>>2) & 0xFF
			out[i*4], out[i*4+1], out[i*4+2], out[i*4+3] = byte(b), byte(g), byte(r), 0xFF
		}
		return out, nil
	case 517: // 16x16 块 RGB565 缩略
		if w%16 != 0 || h%16 != 0 {
			return nil, fmt.Errorf("form517 尺寸非法 %dx%d", w, h)
		}
		if len(data) < w*h/128 {
			return nil, fmt.Errorf("form517 数据不足")
		}
		out := make([]byte, w*h*4)
		for by := 0; by < h/16; by++ {
			for bx := 0; bx < w/16; bx++ {
				idx := (by*w/16 + bx) * 2
				b0, b1 := data[idx], data[idx+1]
				c0 := rgb565(b0, b1)
				for dy := 0; dy < 16; dy++ {
					row := ((by*16+dy)*w + bx*16) * 4
					for dx := 0; dx < 16; dx++ {
						p := row + dx*4
						out[p], out[p+1], out[p+2], out[p+3] = c0[0], c0[1], c0[2], 0xFF
					}
				}
			}
		}
		return out, nil
	case 1026: // DXT3
		return dxt3(data, w, h)
	case 2050: // DXT5
		return dxt5(data, w, h)
	}
	return nil, fmt.Errorf("未知的图片编码格式 form=%d", pi.Form)
}

func rgb565(b0, b1 byte) [3]byte {
	v := uint16(b0) | uint16(b1)<<8
	r5, g6, b5 := v>>11, (v>>5)&0x3F, v&0x1F
	return [3]byte{
		byte((b5<<3 | b5>>2) & 0xFF),
		byte((g6<<2 | g6>>4) & 0xFF),
		byte((r5<<3 | r5>>2) & 0xFF),
	}
}

func encodeARGB(w, h int, bgra []byte) ([]byte, error) {
	if len(bgra) < w*h*4 {
		return nil, fmt.Errorf("像素数据不足: %d < %d", len(bgra), w*h*4)
	}
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		im.Pix[i*4+0] = bgra[i*4+2] // R
		im.Pix[i*4+1] = bgra[i*4+1] // G
		im.Pix[i*4+2] = bgra[i*4+0] // B
		im.Pix[i*4+3] = bgra[i*4+3] // A
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// bmpToPNG 将 BMP 数据转为 PNG。
func bmpToPNG(bmp []byte) ([]byte, error) {
	im, err := bmpDecode(bmp)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---------- DXT 解码 ----------

func dxt3(data []byte, w, h int) ([]byte, error) {
	if len(data) < (w/4+bool2i(w%4>0))*(h/4+bool2i(h%4>0))*16 {
		return nil, fmt.Errorf("dxt3 数据不足")
	}
	out := make([]byte, w*h*4)
	for by := 0; by*4 < h; by++ {
		for bx := 0; bx*4 < w; bx++ {
			off := (by*(w/4+bool2i(w%4>0) ) + bx) * 16
			if off+16 > len(data) {
				continue
			}
			var alpha [16]byte
			for i := 0; i < 8; i++ {
				b := data[off+i]
				alpha[i*2] = b & 0x0F | (b & 0x0F) << 4
				alpha[i*2+1] = b & 0xF0 | (b & 0xF0) >> 4
			}
			colors := dxtColors(data[off+8:])
			for j := 0; j < 4; j++ {
				for i := 0; i < 4; i++ {
					idx := data[off+12+j] >> (i * 2) & 0x03
					px := (by*4+j)*w + bx*4 + i
					if bx*4+i >= w || by*4+j >= h {
						continue
					}
					c := colors[idx]
					out[px*4], out[px*4+1], out[px*4+2], out[px*4+3] = c[0], c[1], c[2], alpha[j*4+i]
				}
			}
		}
	}
	return out, nil
}

func dxt5(data []byte, w, h int) ([]byte, error) {
	bw := w/4 + bool2i(w%4>0)
	if len(data) < bw*(h/4+bool2i(h%4>0))*16 {
		return nil, fmt.Errorf("dxt5 数据不足")
	}
	out := make([]byte, w*h*4)
	for by := 0; by*4 < h; by++ {
		for bx := 0; bx*4 < w; bx++ {
			off := (by*bw + bx) * 16
			if off+16 > len(data) {
				continue
			}
			var alpha [8]byte
			alpha[0], alpha[1] = data[off], data[off+1]
			if alpha[0] > alpha[1] {
				for i := 2; i < 8; i++ {
					alpha[i] = byte(((8-i)*int(alpha[0]) + (i-1)*int(alpha[1]) + 3) / 7)
				}
			} else {
				for i := 2; i < 6; i++ {
					alpha[i] = byte(((6-i)*int(alpha[0]) + (i-1)*int(alpha[1]) + 2) / 5)
				}
				alpha[6], alpha[7] = 0, 255
			}
			colors := dxtColors(data[off+8:])
			cv0 := uint16(data[off+8]) | uint16(data[off+9])<<8
			cv1 := uint16(data[off+10]) | uint16(data[off+11])<<8
			v0le1 := cv0 <= cv1
			for j := 0; j < 4; j++ {
				for i := 0; i < 4; i++ {
					pix := j*4 + i
					// 3 位 alpha 索引
					bit := pix * 3
					byteIdx := off + 2 + bit/8
					sh := bit % 8
					var aIdx byte
					if sh <= 5 {
						aIdx = data[byteIdx] >> sh & 0x07
					} else {
						aIdx = (data[byteIdx]>>sh | data[byteIdx+1]<<(8-sh)) & 0x07
					}
					a := alpha[aIdx]
					// 2 位颜色索引
					cIdx := data[off+12+j] >> (i * 2) & 0x03
					c := colors[cIdx]
					if cIdx == 3 && v0le1 {
						// dxt1 规则下第 4 色为透明黑
						a = 0
					}
					px := (by*4+j)*w + bx*4 + i
					if bx*4+i >= w || by*4+j >= h {
						continue
					}
					out[px*4], out[px*4+1], out[px*4+2], out[px*4+3] = c[0], c[1], c[2], a
				}
			}
		}
	}
	return out, nil
}

// dxtColors 解出一个 block 的两个 565 端点及插值色（第 4 色按 dxt1 规则）。
func dxtColors(b []byte) [4][3]byte {
	c0 := rgb565(b[0], b[1])
	c1 := rgb565(b[2], b[3])
	v0 := uint16(b[0]) | uint16(b[1])<<8
	v1 := uint16(b[2]) | uint16(b[3])<<8
	colors := [4][3]byte{c0, c1}
	if v0 > v1 {
		for i := 0; i < 3; i++ {
			colors[2][i] = byte((int(c0[i])*2 + int(c1[i]) + 1) / 3)
			colors[3][i] = byte((int(c0[i]) + int(c1[i])*2 + 1) / 3)
		}
	} else {
		for i := 0; i < 3; i++ {
			colors[2][i] = byte((int(c0[i]) + int(c1[i])) / 2)
		}
		colors[3] = [3]byte{0, 0, 0}
	}
	return colors
}

func bool2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
