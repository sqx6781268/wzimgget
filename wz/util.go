package wz

import (
	"encoding/binary"
	"fmt"
	"image"
)

// decodeAnsi 解密后的单字节字符串按原字节保留：
// 节点名均为 ASCII；非 ASCII（如 GBK 值）不影响图标提取。
func decodeAnsi(b []byte) string {
	return string(b)
}

// bmpDecode 解码未压缩的 24/32 位 BMP（WZ 中 iconRaw 类数据的常见形式）。
func bmpDecode(data []byte) (image.Image, error) {
	if len(data) < 54 || data[0] != 'B' || data[1] != 'M' {
		return nil, fmt.Errorf("非法 BMP 数据")
	}
	pixOff := int(binary.LittleEndian.Uint32(data[10:]))
	hdrSize := int(binary.LittleEndian.Uint32(data[14:]))
	if hdrSize < 12 || len(data) < 14+hdrSize {
		return nil, fmt.Errorf("非法 BMP 头")
	}
	w := int(int32(binary.LittleEndian.Uint32(data[18:])))
	h := int(int32(binary.LittleEndian.Uint32(data[22:])))
	bpp := binary.LittleEndian.Uint16(data[28:])
	comp := binary.LittleEndian.Uint32(data[30:])
	if comp != 0 {
		return nil, fmt.Errorf("不支持的 BMP 压缩方式 %d", comp)
	}
	flip := false
	if h < 0 {
		h = -h
		flip = true
	}
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("非法 BMP 尺寸 %dx%d", w, h)
	}
	bppBytes := int(bpp / 8)
	stride := ((int(bpp) * w + 31) / 32) * 4
	if pixOff+stride*h > len(data) {
		return nil, fmt.Errorf("BMP 数据不足")
	}
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		src := y * stride
		if !flip {
			src = (h - 1 - y) * stride
		}
		for x := 0; x < w; x++ {
			p := data[pixOff+src+x*bppBytes:]
			o := (y*w + x) * 4
			im.Pix[o+2] = p[0] // B
			im.Pix[o+1] = p[1] // G
			im.Pix[o+0] = p[2] // R
			if bppBytes >= 4 {
				im.Pix[o+3] = p[3]
			} else {
				im.Pix[o+3] = 0xFF
			}
		}
	}
	return im, nil
}
