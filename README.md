# wzimgget — WZ 独立 .img 图标提取工具

扫描游戏客户端 `Data` 目录（由 HaSuite 等工具从 .wz 容器导出的独立 `.img` 文件），
参照 [WzComparerR2](https://github.com/Kagamia/WzComparerR2) 的解析思路，
提取 **装备（Character）、物品（Item）、NPC（Npc）** 三类资源的主图标，
输出为 PNG 并保持与 Data 相同的目录结构。

## 功能特点

- **纯 Go 标准库实现**，无第三方依赖（Go 1.23+）。
- **自动探测加密形式**：内置 BMS / KMS / GMS 三套密钥流（AES-256-ECB 链式密钥流 +
  ANSI `0xAA` / UTF-16 `0xAAAA` 递增掩码）。按 WzComparerR2 `TryDetectEnc` 的思路，
  以根节点类型标记（`Property`/`Canvas` 等）解密结果快速判定密钥，判定失败时
  回退为逐密钥完整试解析。本批 Data 实测为 GMS 密钥。
- **完整属性树解析**：Property / Canvas / Vector2D / Convex2D / Sound_DX8 / UOL /
  RawData 等节点类型，支持变长整数（compint/compfloat/compint64）、
  内联与引用（0x00/0x73/0x01/0x1B）字符串、UOL 跳转。
- **多形态像素解码**：原始 PNG、BMP、zlib（`78 9c`/`78 da` 等）deflate 像素流、
  旧式 AES 分块加密数据；支持 form 1(ARGB4444) / 2(ARGB8888) / 3(缩略) /
  513(RGB565) / 517 / 1026(DXT3) / 2050(DXT5)。
- **图标取值顺序**（仿 WzComparerR2）：
  `info/iconRaw` → `info/icon` → `info/animatedIcon` → `stand0/0` → `stand/0` →
  `strike1/0` → `swingO1/0` → `action/00` → `action/stick0` → `0`；
  某候选解码失败（如空帧）时自动尝试下一候选。

## 构建

```bat
cd wzimgget
go build -o wzimgget.exe .
```

## 使用

### 免参数直接运行（双击）

将 `wzimgget.exe` 放在 `Data` 目录的同级目录下，直接双击运行即可：
自动检测同级 `Data` 目录（忽略大小写），提取图标到同级 `imgdata`，
结束时等待回车以免窗口闪退。

```
游戏目录/
├── Data/            ← 资源目录
├── wzimgget.exe     ← 直接双击运行
└── imgdata/         ← 自动生成，镜像 Data 结构
```

### 批量提取图标

```bat
:: 输出默认为 Data 同级目录下的 imgdata
wzimgget.exe extract D:\game\Data

:: 显式指定输出目录
wzimgget.exe extract D:\game\Data -out D:\icons
```

- 仅处理顶层目录为 `Character` / `Item` / `Npc` 的 `.img`，其余目录整体跳过。
- 输出命名保持原 IMG ID：`Character/Accessory/01142538.img` →
  `imgdata/Character/Accessory/01142538.img.png`。
- 结束后打印统计：总数 / 成功 / 无图标 / 失败。加 `-v` 可逐文件输出明细。
- 说明：`Character/Hair`、`Character/Face` 等穿戴部件的 img 本身不含
  `info/icon` 图标节点，属正常"无图标"跳过。

### 调试命令

```bat
:: 打印 img 属性树（含节点偏移、Canvas 尺寸/格式/数据位置）
wzimgget.exe dump D:\game\Data\Npc\0002000.img

:: 导出指定节点为 PNG（路径用 / 分隔，支持 UOL）
wzimgget.exe png D:\game\Data\Npc\0002000.img stand/0 out.png
```

### 环境变量（调试）

| 变量 | 作用 |
| --- | --- |
| `WZDEBUG=1` | 解析失败时打印失败位置、路径与上下文十六进制 |
| `WZTRACE=1` | 逐 token 打印读取过程（定位解析失步用） |
| `WZSTACK=1` | 失败时额外打印 Go 堆栈 |

## 目录结构

```
wzimgget/
├── main.go          命令行入口（extract / dump / png）
├── extract.go       批量提取：目录遍历、图标候选、统计
├── LICENSE          MIT 许可证
├── .gitignore       忽略编译产物与提取输出
├── docs/            经验文档（格式/加密/像素/业务/踩坑，见 docs/文档索引.md）
└── wz/
    ├── key.go       AES-256-ECB 链式密钥流（BMS/KMS/GMS）
    ├── detect.go    加密形式自动探测（根类型标记判定）
    ├── img.go       img 属性树解析器（含部分树容错）
    ├── node.go      节点模型与查找/路径解析
    ├── canvas.go    Canvas 像素解码与 PNG 编码
    └── util.go      ANSI 解码、手写 BMP 解码（Go 1.27 已移除 image/bmp）
```

## 已知限制

- 仅支持独立 `.img` 文件（无 .wz 文件头/目录树），不支持 `.wz` 容器与 list.wz。
- 密钥固定为官方三套 IV，不支持 ZLZ 动态密钥。
- 极少数 NPC（如 9330077）所有帧均为空画布（数据仅 2 字节），无法产出图像。

## 许可证与免责声明

- 本项目采用 [MIT License](LICENSE) 开源。
- 本项目仅用于游戏资源文件格式的学习与研究，不附带任何游戏数据；
  游戏资源（含 .wz/.img 内的图像、文本等）的版权归原游戏厂商及相关权利人所有，
  请勿将其用于商业用途或再次分发。

