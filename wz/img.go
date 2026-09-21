package wz

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"runtime/debug"
	"strings"
)

// Img 表示一个独立 .img 文件解析后的属性树。
type Img struct {
	Data []byte
	Ks   *Keystream
	Root *Node
}

type parser struct {
	buf   []byte
	pos   int
	ks    *Keystream
	limit int      // 当前子 img 的结束位置（eob）
	path  []string // 调试用：当前节点路径
}

// Load 解析独立 .img 文件内容，自动探测加密方式（BMS/KMS/GMS）。
// 优先按 WzComparerR2 TryDetectEnc 的思路以根类型标记快速判定密钥；
// 判定失败时回退为逐密钥完整试解析。即使解析中途出错，
// 也会尽力返回已成功解析的最深部分属性树。
func Load(data []byte) (*Img, error) {
	if ks, _, ok := DetectKeystream(data); ok {
		if img, _, err := loadWith(data, ks); err == nil {
			return img, nil
		}
	}
	var lastErr error
	var best *Img
	bestProg := -1
	for _, iv := range [][4]byte{IV_KMS, IV_GMS, IV_BMS} {
		ks := NewKeystream(iv)
		img, prog, err := loadWith(data, ks)
		if err == nil {
			return img, nil
		}
		if prog > bestProg && img != nil {
			best, bestProg = img, prog
		}
		lastErr = err
	}
	return best, fmt.Errorf("解析失败（加密探测未通过）: %w", lastErr)
}

func loadWith(data []byte, ks *Keystream) (img *Img, prog int, err error) {
	root := &Node{Name: "root", Kind: KindSub}
	img = &Img{Data: data, Ks: ks, Root: root}
	p := &parser{buf: data, ks: ks, limit: len(data)}
	defer func() {
		if r := recover(); r != nil {
			if os.Getenv("WZDEBUG") != "" {
				fmt.Fprintf(os.Stderr, "[%s] 失败位置 pos=%d 路径=%v: %v\n", ksName(ks), p.pos, p.path, r)
				lo := p.pos - 32
				if lo < 0 {
					lo = 0
				}
				hi := p.pos + 32
				if hi > len(p.buf) {
					hi = len(p.buf)
				}
				fmt.Fprintf(os.Stderr, "上下文 %d..%d: % x\n", lo, hi, p.buf[lo:hi])
				if os.Getenv("WZSTACK") != "" {
					debug.PrintStack()
				}
			}
			prog = p.pos
			err = fmt.Errorf("%v", r)
		}
	}()
	p.extractImg(root, len(data))
	return img, p.pos, nil
}

func ksName(ks *Keystream) string {
	switch ks.iv {
	case IV_KMS:
		return "KMS"
	case IV_GMS:
		return "GMS"
	}
	return "BMS"
}

// ---------- 基础读取 ----------

// trace 输出逐 token 跟踪（WZTRACE=1 时启用），用于定位解析失步。
func (p *parser) trace(format string, args ...any) {
	if os.Getenv("WZTRACE") == "" {
		return
	}
	prefix := strings.Join(p.path, "/")
	if prefix == "" {
		prefix = "(根)"
	}
	detail := ""
	hi := p.pos + 16
	if hi > len(p.buf) {
		hi = len(p.buf)
	}
	if p.pos <= len(p.buf) {
		detail = fmt.Sprintf(" % x", p.buf[p.pos:hi])
	}
	fmt.Fprintf(os.Stderr, "@%-6d [%s] %s%s\n", p.pos, prefix, fmt.Sprintf(format, args...), detail)
}

func (p *parser) need(n int) {
	if p.pos+n > len(p.buf) {
		panic(fmt.Sprintf("越界读取: pos=%d need=%d len=%d", p.pos, n, len(p.buf)))
	}
}

func (p *parser) u8() byte {
	p.need(1)
	b := p.buf[p.pos]
	p.pos++
	return b
}

func (p *parser) skip(n int) {
	p.need(n)
	p.pos += n
}

func (p *parser) fixedI32() int32 {
	p.need(4)
	v := int32(binary.LittleEndian.Uint32(p.buf[p.pos:]))
	p.pos += 4
	return v
}

func (p *parser) fixedI16() int16 {
	p.need(2)
	v := int16(binary.LittleEndian.Uint16(p.buf[p.pos:]))
	p.pos += 2
	return v
}

// readCompInt 变长整数：首字节为 -128 时后跟 int32（WzComparerR2 的 ReadInt32）。
func (p *parser) readCompInt() int64 {
	s := int8(p.u8())
	if s == -128 {
		return int64(p.fixedI32())
	}
	return int64(s)
}

func (p *parser) readCompInt64() int64 {
	s := int8(p.u8())
	if s == -128 {
		p.need(8)
		v := int64(binary.LittleEndian.Uint64(p.buf[p.pos:]))
		p.pos += 8
		return v
	}
	return int64(s)
}

func (p *parser) readCompFloat() float64 {
	s := int8(p.u8())
	if s == -128 {
		p.need(4)
		v := math.Float32frombits(binary.LittleEndian.Uint32(p.buf[p.pos:]))
		p.pos += 4
		return float64(v)
	}
	return float64(s)
}

// ---------- 字符串 ----------

// readStringInline 读取并解密一个内联字符串。
func (p *parser) readStringInline() string {
	at := p.pos
	size := int(int8(p.u8()))
	s := ""
	switch {
	case size < 0:
		if size == -128 {
			size = int(p.fixedI32())
		} else {
			size = -size
		}
		p.need(size)
		b := append([]byte(nil), p.buf[p.pos:p.pos+size]...)
		p.pos += size
		p.ks.Decrypt(b)
		mask := byte(0xAA)
		for i := range b {
			b[i] ^= mask
			mask++
		}
		s = decodeAnsi(b)
	case size > 0:
		if size == 127 {
			size = int(p.fixedI32())
		}
		p.need(size * 2)
		b := append([]byte(nil), p.buf[p.pos:p.pos+size*2]...)
		p.pos += size * 2
		p.ks.Decrypt(b)
		mask := uint16(0xAAAA)
		runes := make([]rune, size)
		for i := 0; i < size; i++ {
			v := binary.LittleEndian.Uint16(b[i*2:]) ^ mask
			mask++
			runes[i] = rune(v)
		}
		s = string(runes)
	}
	p.trace("str@%d size=%d => %q", at, size, s)
	return s
}

func (p *parser) readStringAt(off int) string {
	old := p.pos
	p.pos = off
	s := p.readStringInline()
	p.pos = old
	return s
}

// readStringTag 读取带类型标记的字符串（标记字节不加密）。
func (p *parser) readStringTag() (marker byte, s string) {
	at := p.pos
	marker = p.u8()
	switch marker {
	case 0x00, 0x73:
		s = p.readStringInline()
	case 0x01, 0x1B:
		off := int(p.fixedI32())
		s = p.readStringAt(off)
		p.trace("strtag@%d 0x%02x off=%d => %q", at, marker, off, s)
		return
	case 0x04:
		p.skip(8)
		s = ""
	default:
		panic(fmt.Sprintf("非法字符串标记 0x%02x @%d", marker, at))
	}
	p.trace("strtag@%d 0x%02x => %q", at, marker, s)
	return
}

// ---------- 树解析（对照 WzComparerR2 v1.0 ExtractImg/ExtractValue） ----------

func (p *parser) extractImg(parent *Node, eob int) {
	_, tag := p.readStringTag()
	p.trace("img tag=%q eob=%d", tag, eob)
	switch tag {
	case "Property":
		parent.Kind = KindSub
		p.skip(2)
		n := p.readCompInt()
		p.trace("Property count=%d", n)
		for i := 0; i < int(n); i++ {
			p.extractValue(parent, eob)
		}

	case "Shape2D#Vector2D":
		parent.Kind = KindVector
		parent.X = int32(p.readCompInt())
		parent.Y = int32(p.readCompInt())

	case "Canvas":
		p.skip(1)
		head := p.u8()
		p.trace("Canvas subprop头=%d", head)
		if head == 0x01 {
			p.skip(2)
			n := p.readCompInt()
			p.trace("Canvas 子属性 count=%d", n)
			for i := 0; i < int(n); i++ {
				p.extractValue(parent, eob)
			}
		}
		w := int(p.readCompInt())
		h := int(p.readCompInt())
		form := int(p.readCompInt()) + int(int8(p.u8()))
		p.skip(4)
		bufsize := int(p.fixedI32())
		p.trace("Canvas %dx%d form=%d bufsize=%d dataoff=%d", w, h, form, bufsize, p.pos+1)
		parent.Kind = KindCanvas
		parent.Png = &PngInfo{W: w, H: h, Form: form, Off: p.pos + 1, Length: bufsize - 1}
		p.skip(bufsize)

	case "Shape2D#Convex2D":
		parent.Kind = KindConvex
		n := p.readCompInt()
		for i := 0; i < int(n); i++ {
			child := &Node{Name: fmt.Sprintf("%d", i)}
			p.extractImg(child, eob)
			parent.Children = append(parent.Children, child)
		}

	case "Sound_DX8":
		p.skip(1)
		l := int(p.readCompInt())
		p.readCompInt() // 时长
		parent.Kind = KindSound
		parent.RawOff = eob - l
		parent.RawLen = l
		p.pos = eob

	case "UOL":
		p.skip(1)
		parent.Kind = KindUOL
		_, parent.S = p.readStringTag()

	case "RawData": // 较新版本格式
		ver := p.u8()
		if ver == 1 && p.u8() == 0x01 {
			p.skip(2)
			n := p.readCompInt()
			for i := 0; i < int(n); i++ {
				p.extractValue(parent, eob)
			}
		}
		l := int(p.readCompInt())
		if parent.Kind == KindSub && len(parent.Children) > 0 {
			// 带子属性的 RawData，数据仍紧随其后
		}
		parent.RawOff = p.pos
		parent.RawLen = l
		p.skip(l)

	default:
		panic(fmt.Sprintf("未知 wz tag: %q @%d", tag, p.pos))
	}
}

func (p *parser) extractValue(parent *Node, eob int) {
	at := p.pos
	_, name := p.readStringTag()
	nameNode := &Node{Name: name, AtOff: at}
	parent.Children = append(parent.Children, nameNode)
	p.path = append(p.path, name)
	defer func() { p.path = p.path[:len(p.path)-1] }()
	flag := p.u8()
	p.trace("value name=%q flag=0x%02x", name, flag)
	switch flag {
	case 0x00:
		nameNode.Kind = KindNull
	case 0x02, 0x0B:
		nameNode.Kind = KindInt
		nameNode.I = int64(p.fixedI16())
	case 0x03, 0x13:
		nameNode.Kind = KindInt
		nameNode.I = p.readCompInt()
	case 0x14:
		nameNode.Kind = KindInt
		nameNode.I = p.readCompInt64()
	case 0x04:
		nameNode.Kind = KindFloat
		nameNode.F = p.readCompFloat()
	case 0x05:
		nameNode.Kind = KindDouble
		p.need(8)
		nameNode.F = math.Float64frombits(binary.LittleEndian.Uint64(p.buf[p.pos:]))
		p.pos += 8
	case 0x08:
		nameNode.Kind = KindString
		_, nameNode.S = p.readStringTag()
	case 0x09:
		subEob := int(p.fixedI32()) + p.pos
		p.trace("进入子img eob=%d", subEob)
		p.extractImg(nameNode, subEob)
	default:
		panic(fmt.Sprintf("未知值类型 0x%02x @%d", flag, p.pos-1))
	}
}

// Resolve 按斜杠路径在当前树中查找节点（支持 UOL 单层跳转）。
func (n *Node) Resolve(path string) *Node {
	cur := n
	for _, seg := range strings.Split(path, "/") {
		if seg == "" {
			continue
		}
		if cur.Kind == KindUOL {
			cur = n.Resolve(cur.S)
		}
		child := cur.Child(seg)
		if child == nil {
			return nil
		}
		cur = child
	}
	return cur
}

// FollowUOL 若节点是 UOL，则解析其指向的节点。
func (root *Node) FollowUOL(n *Node) *Node {
	for i := 0; n != nil && n.Kind == KindUOL && i < 16; i++ {
		n = root.Resolve(n.S)
	}
	return n
}
