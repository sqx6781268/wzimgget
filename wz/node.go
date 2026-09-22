package wz

// 节点类型，对应 WZ img 属性树中的值类型。
type Kind int

const (
	KindNull   Kind = iota // 空
	KindInt                // 整数
	KindFloat              // 单精度
	KindDouble             // 双精度
	KindString             // 字符串
	KindSub                // 子属性树
	KindVector             // 坐标
	KindCanvas             // 画布（图片）
	KindConvex             // 凸多边形
	KindSound              // 声音
	KindUOL                // 引用链接
	KindRaw                // 原始数据块
)

func (k Kind) String() string {
	switch k {
	case KindNull:
		return "null"
	case KindInt:
		return "int"
	case KindFloat:
		return "float"
	case KindDouble:
		return "double"
	case KindString:
		return "string"
	case KindSub:
		return "sub"
	case KindVector:
		return "vector"
	case KindCanvas:
		return "canvas"
	case KindConvex:
		return "convex"
	case KindSound:
		return "sound"
	case KindUOL:
		return "uol"
	case KindRaw:
		return "raw"
	}
	return "?"
}

// Node 是属性树中的一个节点。
type Node struct {
	Name     string
	AtOff    int // 节点名标记在文件中的偏移（调试用）
	Kind     Kind
	I        int64
	F        float64
	S        string
	X, Y     int32
	Png      *PngInfo
	RawOff   int
	RawLen   int
	Children []*Node
}

// PngInfo 描述 Canvas 节点中的图片数据位置与编码方式。
type PngInfo struct {
	W, H   int
	Form   int // 0 或负数表示未压缩原始数据；1/2/3/513/517/1026/2050 为各种像素编码
	Scale  int
	Off    int
	Length int
}

func (n *Node) Child(name string) *Node {
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}
