# WZ 独立 .img 文件格式

> 适用对象：由 HaSuite 等工具从 .wz 容器导出的**独立 .img 文件**（客户端资源 Data 目录即此形态）。
> 参考实现：[WzComparerR2](https://github.com/Kagamia/WzComparerR2)（v1.0 master 与 v80315 两版均核对过）。

## 1. 与 .wz 容器的区别

| 形态 | 结构 |
| --- | --- |
| .wz 容器 | 文件头（校验和/FStart/版权）+ 目录树（checkSum/offset/size 索引）+ 各 img 数据段 + 尾部加密块 |
| 独立 .img | **仅一个 img 数据段**：文件第 0 字节起就是属性树根，无头、无目录索引、无校验 |

因此独立 .img 中所有"绝对偏移"（0x01/0x1B 字符串引用、0x09 子 img 长度基准）都以**文件起始为基准**，
等价于 WzComparerR2 中 `StringReferenceOffsetBytes = 0` 的情形。

## 2. 根节点

文件开头是一个带标记的内联字符串，即 img 类型标记：

```
73 f8 <8字节密文>        ; 0x73 标记 + 长度 -8 + "Property" 的密文
```

- 标记 `0x73`：内联字符串（对象类型名专用）；
- 标记 `0x1B`：后跟 int32，指向文件内绝对偏移处的字符串；
- 合法类型名集合：`Property`、`Canvas`、`Shape2D#Vector2D`、`Shape2D#Convex2D`、
  `Sound_DX8`、`UOL`、`RawData`（GMS v243+）、`Canvas#Video`（KMST v1181+）。
  **根标记解密后是否落在此集合内，是自动探测加密形式的关键依据**（见 加解密与自动探测.md）。

## 3. 变长编码

| 编码 | 规则 |
| --- | --- |
| compint (int32) | 首字节 sbyte；`-128` → 后跟 fixed int32；否则即值 |
| compint64 | 首字节 sbyte；`-128` → 后跟 fixed int64 |
| compfloat | 首字节 sbyte；`-128` → 后跟 fixed float32 |
| 字符串长度 | 首字节 sbyte：负=ANSI 字节数（取反），`-128`→后跟 int32；正=UTF-16LE 字符数，`127`→后跟 int32 |

## 4. 带标记字符串（不加密的标记字节）

| 标记 | 含义 | 出现位置 |
| --- | --- | --- |
| `0x00` | 内联字符串 | 属性名、UOL 路径、字符串值 |
| `0x73` | 内联字符串 | 对象类型名 |
| `0x01` | int32 → 文件内绝对偏移处的字符串 | 属性名等 |
| `0x1B` | int32 → 同上（类型名版） | 对象类型名 |
| `0x04` | int32+int32 共 8 字节占位，值为空 | 偶见 |

引用字符串（0x01/0x1B）指向的"字符串池"条目本身仍是完整的内联格式（长度字节+密文），
**每个字符串独立从密钥流下标 0 解密**（不是按文件绝对位置索引密钥流）。

## 5. 值类型 flag（Property 子项）

`extractValue` = 字符串(属性名) + 1 字节 flag + 值：

| flag | 类型 | 读取 |
| --- | --- | --- |
| `0x00` | null | 无 |
| `0x02` / `0x0B` | int16 | fixed int16（0x0B 为较新版本出现） |
| `0x03` / `0x13` | int32 | compint |
| `0x14` | int64 | compint64 |
| `0x04` | float | compfloat |
| `0x05` | double | fixed double |
| `0x08` | 字符串 | 带标记字符串（0x00/0x01/0x04） |
| `0x09` | 子 img | fixed int32 长度 len；`eob = 当前pos + len`，递归解析到 eob |

## 6. 各类型节点主体

### Property（子目录）
```
[0x00 两字节填充] count=compint ; 循环 count 次 extractValue
```

### Shape2D#Vector2D
```
x=compint y=compint
```

### Canvas（旧 v80315 布局，样本数据实测为此版）
```
skip 1 字节
if readByte()==0x01:            ; 有子属性
    skip 2; count=compint; 循环 extractValue
w=compint  h=compint
form=compint + sbyte            ; 组合规则见 画布像素格式与解码.md
skip 4                          ; 旧版为 scale(1)+unknown(3)，新版布局不同
bufsize=fixed int32
像素数据起点 = pos+1，长度 = bufsize-1   ; 首字节为标志位，跳过
skip bufsize
```
> 新版（KMST1186+）布局为 `w,h,form=compint,scale=u8,pages=compint,unknown1=compint,skip2,dataLen=int32`，
> 数据紧跟无 +1 偏移。混用会导致整棵树失步，先用样例文件 dump 验证布局。

### Shape2D#Convex2D
```
count=compint ; 循环 count 次 extractImg（每项必须是 Vector2D）
```

### UOL
```
skip 1; 带标记字符串 = 跳转路径（如 "Character/Hair/_Canvas/xxx.img/default/hairOverHead" 或相对路径）
```
UOL 是软链接，取值时需按路径解析（注意可能多级跳转，加次数上限防环）。

### Sound_DX8 / RawData
- Sound（旧）：`skip1; dataLen=compint; duration=compint;` 数据在 `eob-dataLen`。
- RawData（新）：`ver=u8; ver==1 且下一字节 0x01 则有子属性; dataLen=compint;` 数据紧跟。

## 7. 解析容错建议

- 用 panic/recover 包住整棵树解析，出错时**保留已挂载的部分树**（先 append 节点再解析值），
  便于 dump 定位与"尽力提取"。
- 记录每个节点的 `AtOff`（属性名标记起始偏移），dump 时打印，与 hexdump 直接对照排查失步。
- 逐 token 跟踪日志（位置+标记+解码结果）是定位失步最有效的手段（见 调试经验与踩坑记录.md）。
