# wzimgget — WZ 独立 .img 图标提取工具

扫描游戏客户端 `Data` 目录（由 HaSuite 等工具从 .wz 容器导出的独立 `.img` 文件），
参照 [WzComparerR2](https://github.com/Kagamia/WzComparerR2) 的解析思路，
提取 **装备（Character）、物品（Item）、NPC（Npc）、怪物（Mob）、技能（Skill）、
变身（Morph）、地图物件（Reactor）** 七类资源的主图标，
输出为 PNG 并保持与 Data 相同的目录结构。

## 功能特点

- **纯 Go 标准库实现**，无第三方依赖（Go 1.23+）。
- **自动探测加密形式**：内置 BMS / KMS / GMS 三套密钥流（AES-256-ECB 链式密钥流 +
  ANSI `0xAA` / UTF-16 `0xAAAA` 递增掩码）。按 WzComparerR2 `TryDetectEnc` 的思路，
  以根节点类型标记（`Property`/`Canvas` 等）解密结果快速判定密钥，判定失败时
  回退为逐密钥完整试解析。本批 Data 实测为 GMS 密钥。
- **外部密钥与暴力枚举兜底**：`-key` 可传入自定义 IV（8 位十六进制）或 32 字节用户
  密钥（64 位十六进制，ZLZ 动态密钥场景）；内置与外部密钥全部失败时，自动对
  0..2^32-1 全 IV 空间并行暴力枚举（两级判据，见 docs），命中后记录该密钥供本次
  运行后续文件优先使用，并写入 `wzimgget_keys.log`；`-nobf` 可关闭该兜底。
- **完整属性树解析**：Property / Canvas / Vector2D / Convex2D / Sound_DX8 / UOL /
  RawData 等节点类型，支持变长整数（compint/compfloat/compint64）、
  内联与引用（0x00/0x73/0x01/0x1B）字符串、UOL 跳转。
- **多形态像素解码**：原始 PNG、BMP、zlib（`78 9c`/`78 da` 等）deflate 像素流、
  旧式 AES 分块加密数据；支持 form 1(ARGB4444) / 2(ARGB8888) / 3(缩略) /
  513(RGB565) / 517 / 1026(DXT3) / 2050(DXT5)。
- **图标取值顺序**（仿 WzComparerR2）：
  `info/iconRaw` → `info/icon` → `info/animatedIcon` → `iconRaw` → `icon` →
  `iconRaw/0` → `icon/0` → `stand0/0` → `stand/0` → `strike1/0` → `swingO1/0` →
  `action/00` → `action/stick0` → `0`；
  某候选解码失败（如空帧）时自动尝试下一候选。
  NPC/怪物/变身/物件等非物品资源的图标通常不在 `info/icon`，而在 `stand/0`
  （或 Reactor 的 `0/0`）动画首帧；
  Item 四位数组文件（如 `Etc/0400.img`、`Cash/0501.img`）会**按内部物品 ID 逐个导出**
  （如 `04000000.img.png`，与组文件同目录），而不是只出一张组文件图。
- **Hair/Face 代表图开关**：`Character/Hair`、`Character/Face` 的穿戴部件本身无图标节点，
  默认按"无图标"跳过；加 `-hairface` 参数后改为导出其代表图
  （`default/hairOverHead` / `default/face`）。
- **Store 模式打包**：批量提取完成后自动打包为不压缩的 `imgdata.zip`，
  消除海量小文件的 NTFS 簇浪费，便于备份传输、读取零解压；支持 `pack` 子命令单独打包。

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

:: 传入外部密钥（逗号分隔多个）：8位十六进制=自定义IV；64位十六进制=32字节用户密钥
wzimgget.exe extract D:\game\Data -key 4D23C72B

:: 关闭 IV 暴力枚举兜底（默认开启：所有密钥失败时自动全空间搜索）
wzimgget.exe extract D:\game\Data -nobf

:: 同时导出 Hair/Face 穿戴部件代表图（默认跳过）
wzimgget.exe extract D:\game\Data -hairface
```

- 仅处理顶层目录为 `Character` / `Item` / `Npc` / `Mob` / `Skill` / `Morph` / `Reactor`
  的 `.img`，其余目录整体跳过。
- 输出命名保持原 IMG ID：`Character/Accessory/01142538.img` →
  `imgdata/Character/Accessory/01142538.img.png`；
  Item 组文件（如 `Item/Etc/0400.img`）按内部物品 ID 输出：
  `imgdata/Item/Etc/04000000.img.png`（一图一物品，可直接按物品 ID 匹配）。
- 结束后打印统计：总数 / 成功 / 无图标 / 失败。加 `-v` 可逐文件输出明细。
- 说明：`Character/Hair`、`Character/Face` 等穿戴部件的 img 本身不含
  `info/icon` 图标节点，属正常"无图标"跳过。

### 提取单个 img 文件

```bat
:: 默认输出到文件同目录的 <文件名>.img.png
wzimgget.exe img D:\game\Data\Npc\0002000.img

:: 指定输出 PNG 路径（或目录）
wzimgget.exe img D:\game\Data\Npc\0002000.img -out icon.png
```

支持同样的 `-key` / `-nobf` 选项；未知密钥文件同样会触发暴力枚举兜底。
对 Item 组文件（多物品 ID），所有图标按 `<物品ID>.img.png` 输出，
此时 `-out` 应给目录（给 .png 文件名会被忽略并提示）。

### Store 模式打包

批量提取结束后会自动把输出目录打包为不压缩的 `imgdata.zip`
（PNG 已自带压缩，Store 模式零解压读取、消除海量小文件的 NTFS 簇浪费，便于备份与传输）：

```bat
:: 单独打包任意目录（默认输出 <目录>.zip）
wzimgget.exe pack D:\game\imgdata

:: 指定输出 ZIP 路径
wzimgget.exe pack D:\game\imgdata -out E:\backup\icons-2026.zip

:: 提取后不自动打包
wzimgget.exe extract D:\game\Data -nozip
```

归档内为与原目录一致的 `/` 分隔相对路径；目录内容再有增删时必须重新打包。

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
├── main.go          命令行入口（extract / img / dump / png）
├── extract.go       批量提取：目录遍历、图标候选、统计
├── keys.go          外部密钥注册与暴力枚举兜底编排、密钥日志
├── pack.go          Store 模式 ZIP 打包（extract 自动执行 / pack 子命令）
├── LICENSE          MIT 许可证
├── .gitignore       忽略编译产物与提取输出
├── docs/            经验文档（格式/加密/像素/业务/踩坑/密钥对照，见 docs/文档索引.md）
└── wz/
    ├── key.go       AES-256-ECB 链式密钥流（BMS/KMS/GMS + 外部密钥）
    ├── detect.go    加密形式自动探测（根类型标记判定）
    ├── bruteforce.go IV 全空间暴力枚举兜底（两级判据、多核并行）
    ├── img.go       img 属性树解析器（含部分树容错）
    ├── node.go      节点模型与查找/路径解析
    ├── canvas.go    Canvas 像素解码与 PNG 编码
    └── util.go      ANSI 解码、手写 BMP 解码（Go 1.27 已移除 image/bmp）
```

## 已知限制

- 仅支持独立 `.img` 文件（无 .wz 文件头/目录树），不支持 `.wz` 容器与 list.wz。
- 默认内置官方三套 IV；ZLZ 动态密钥或私有 IV 可用 `-key` 外部传入，
  或交由暴力枚举兜底自动搜索（约 2^32 次 AES 运算，本机实测最坏 ~15 秒）。
- 极少数 NPC（如 9330077）所有帧均为空画布（数据仅 2 字节），无法产出图像。

## 许可证与免责声明

- 本项目采用 [MIT License](LICENSE) 开源，版本变更记录见 [CHANGELOG.md](CHANGELOG.md)。
- 本项目仅用于游戏资源文件格式的学习与研究，不附带任何游戏数据；
  游戏资源（含 .wz/.img 内的图像、文本等）的版权归原游戏厂商及相关权利人所有，
  请勿将其用于商业用途或再次分发。

