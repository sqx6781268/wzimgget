# 更新日志

本项目版本号遵循语义化版本，标签格式为 `vX.Y.Z`。

## v1.2.0（2026-09-23）

**新增**
- Store 模式打包：`extract` 完成后自动把输出目录打包为不压缩的 `imgdata.zip`
  （PNG 已自带压缩，Store 模式零解压读取、消除海量小文件的 NTFS 簇浪费）；
  新增 `pack <目录> [-out x.zip]` 子命令单独打包；`-nozip` 关闭自动打包。
- 导出范围由四类扩为七类资源域：新增 技能 `Skill`（`info/icon`）、
  变身 `Morph`（`stand/0`，根级落空时下探一层取一张，如 `fly/0`）、
  地图物件 `Reactor`（`0/0` 动画首帧）。
- Item 四位数组文件（如 `Etc/0400.img`、`Cash/0501.img`）改为**按内部物品 ID
  逐个导出**（`04000000.img.png`，与组文件同目录），可直接按实际物品 ID 匹配；
  此前每个组文件只出一张、且 Special 变体（`<ID>/iconRaw/0`、`<ID>/icon`）会遗漏。
- `-hairface` 开关：导出 Hair/Face 穿戴部件代表图
  （`default/hairOverHead` / `default/face`），默认仍按"无图标"跳过。

**修复**
- UOL 指向缺失路径时 `Resolve` 空指针 panic（实测 Morph/0098 触发）。

**数据**
- 全量实测：56991 个 img → 成功 30983，共 37110 张图标；
  `imgdata.zip` 37110 个文件 63.3MB（源目录磁盘占用 131MB）。

## v1.1.0（2026-09-22）

**新增**
- `-key` 外部密钥：逗号分隔多个，8 位十六进制 = 自定义 IV，
  64 位十六进制 = 32 字节 AES 用户密钥（ZLZ 动态密钥场景），优先于内置密钥。
- IV 暴力枚举兜底：外部与内置密钥全部失败时，自动对 0..2^32-1 全 IV 空间
  多核并行枚举（两级判据：根类型标记签名快筛 + 完整解密验证）；
  命中后记录密钥供本次运行后续文件优先使用，并写入 `wzimgget_keys.log`；
  `-nobf` 可关闭。全空间最坏耗时约 14 秒（8 核）。
- `img <xx.img> [-out 输出]` 子命令：解密提取单个文件的主图标。
- 文档：《WZ密钥与版本对照及暴力枚举兜底说明》（上游 Harepacker/MapleLib 规则整理）。

## v1.0.0（2026-09-22）

**首个二进制发布（Windows x64/x86，纯静态）**
- 独立 `.img` 全属性树解析（Property/Canvas/Vector2D/Convex2D/Sound_DX8/UOL/RawData），
  AES-256-ECB 链式密钥流 + BMS/KMS/GMS 加密形式自动探测（TryDetectEnc 思路）。
- 多形态像素解码：PNG/BMP/zlib 直通与 form 1/2/3/513/517/1026(DXT3)/2050(DXT5)。
- 批量提取装备/物品/NPC 图标，镜像目录结构，保持 IMG ID 命名；
  免参数双击运行（自动识别同级 `Data` 目录）。

## 初始提交（2026-09-21）

- Go 实现的 WZ 独立 .img 图标提取工具开源，参照 WzComparerR2 解析思路；
  附中文经验文档（格式/加解密/像素解码/业务规则/踩坑记录）。
