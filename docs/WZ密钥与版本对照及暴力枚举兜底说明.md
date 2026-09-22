# MapleStory WZ 密钥 / 版本对照 / 暴力枚举兜底 —— 经验总结

> 来源：GitHub `lastbattle/Harepacker-resurrected`（分支 master）及其子模块
> `lastbattle/MapleLib`（分支 main）。仅在网页端读取源码整理，未克隆。
>
> 结论：上游**没有**"每个客户端版本各一把 AES 密钥"的字典。WZ 用**同一把全局用户密钥**
> + **按区域区分的 4 字节 IV**。版本差异体现在：区域 IV、patch 版本号头（算 versionHash
> 用于偏移混淆）、极少数私有端 CUSTOM IV。全部失败时用**全 32 位 IV 暴力枚举**兜底。

---

## 1. 全局 AES 用户密钥（所有版本通用）
文件：`MapleLib/MapleCryptoLib/MapleCryptoConstants.cs`

`MAPLESTORY_USERKEY_DEFAULT`：128 字节（32 个 DWORD）。真正喂 AES-256 的是裁剪版
`GetTrimmedUserKey`：每隔 16 字节取首字节（key[0]=U[0], key[4]=U[16], …, key[28]=U[112]）。

裁剪得到的 32 字节实际密钥（即经典 GMS WZ 密钥 `{13,08,06,B4,1B,0F,33,52}` 的 DWORD 展开）：
```
13 00 00 00  08 00 00 00  06 00 00 00  B4 00 00 00
1B 00 00 00  0F 00 00 00  33 00 00 00  52 00 00 00
```
128 字节原表（每行 16 字节，按上面规则取每行每 DWORD 首字节即得上表）：
```
13 00 00 00 52 00 00 00 2A 00 00 00 5B 00 00 00
08 00 00 00 02 00 00 00 10 00 00 00 60 00 00 00
06 00 00 00 02 00 00 00 43 00 00 00 0F 00 00 00
B4 00 00 00 4B 00 00 00 35 00 00 00 05 00 00 00
1B 00 00 00 0A 00 00 00 5F 00 00 00 09 00 00 00
0F 00 00 00 50 00 00 00 0C 00 00 00 1B 00 00 00
33 00 00 00 55 00 00 00 01 00 00 00 09 00 00 00
52 00 00 00 DE 00 00 00 C7 00 00 00 1E 00 00 00
```
其它常量：`bDefaultAESKeyValue = C6 50 53 F2 A8 42 9D 7F 77 09 1D 26 42 53 88 7C`（包加密用），
`bShuffle[256]`（包加密 IV 洗牌表，非 WZ）。

---

## 2. 区域 IV 与"版本 → IV"实测对照
文件：`MapleLib/WzLib/WzAESConstant.cs`
```
WZ_GMSIV      = 4D 23 C7 2B   // GMS
WZ_MSEAIV     = B9 7D 63 E9   // 新版 GMS / MSEA / KMS / EMS
WZ_BMSCLASSIC = 00 00 00 00   // BMS / CLASSIC（密钥流全 0，等价不混淆）
WZ_OffsetConstant = 0x581C3F6D
```
分派（`MapleLib/WzLib/Util/WzTool.cs → GetIvByMapleVersion`，第 78 行）：
```
EMS → WZ_MSEAIV | GMS → WZ_GMSIV | CUSTOM → 读配置 GetCusomWzIVEncryption()
GENERATE → 全0（只新建不读） | BMS/CLASSIC/default → WZ_BMSCLASSIC
```
实测版本→IV（来自 `docs/perf/wz-key-bruteforce.md` "Correctness checks"，均用 TamingMob.wz）：

| 客户端版本 | 地区 | 实测 IV | 常量 |
|---|---|---|---|
| Taiwan v113 | MSEA/TW | B9 7D 63 E9 | WZ_MSEAIV |
| MSEA v82 | MSEA | B9 7D 63 E9 | WZ_MSEAIV |
| Global v95 "Ariku" | GMS | 4D 23 C7 2B | WZ_GMSIV |
| Global v146 | GMS（偏移全0） | 00 00 00 00 | WZ_BMSCLASSIC |

→ 覆盖"更多版本"本质是按这 3 类区域 IV 归类 + CUSTOM + 兜底枚举。

版本号头 / 64 位客户端（`MapleLib/WzLib/WzFile.cs` 第 29-35、312-344 行）：
- 32 位老端：头 0x3C 有 2 字节 encVer，可枚举 patch 版本试解。
- KMST1132 / GMSv230（约 2022-02-09）起 64 位端**移除**该 2 字节，固定用 `777`；读取在
  `770~779`（wzVersionHeader64bit_start）区间试确定 versionHash。
- versionHash 由 `CheckAndGetVersionHash(encVer, patchVer)` 算，只用于**偏移**解密，非 AES 密钥。

---

## 3. WZ 密钥流（可变密钥）生成算法
文件：`MapleLib/WzLib/Util/WzMutableKey.cs → EnsureKeySize`
```
if (BitConverter.ToInt32(iv)==0) { keys = new byte[size]; return }   // IV 全0 -> 密钥流全0
aes=Aes.Create(); aes.KeySize=256; aes.Mode=ECB; aes.Padding=None; aes.Key=trimmedUserKey
enc=aes.CreateEncryptor()
for i in range(0,size,16):
    if i==0: block = iv[j%4] 铺满 16 字节          // 种子块 = 4字节IV循环
    else:    block = keys[i-16:i]                   // 上一块密文当下一块明文（串联）
    keys[i:i+16] = AES_ECB_Encrypt(block)
```
要点：换 IV → 整条密钥流不同（这就是区域 IV 生效原因）。
封装入口 `MapleLib/WzLib/Util/WzKeyGenerator.cs`：
`GenerateWzKey(iv)` 用 UserKey_WzLib；`GenerateWzKey(iv,128字节)` 先裁剪；
`GenerateLuaWzKey()` 固定 WZ_MSEAIV + 默认裁剪密钥（解 .Lua 属性）。

---

## 4. ZLZ 通道（GETFROMZLZ）
`MapleLib/WzLib/Util/WzKeyGenerator.cs`
```
GetIvFromZlz:     seek 0x10040 读 4 字节                       -> IV
GetAesKeyFromZlz: seek 0x10060，循环8次：读4字节再跳12字节      -> 32 字节 AES key
```
当同目录存在 `ZLZ.dll` 且自动检测成功率过低时走此路。

---

## 5. 自动版本检测（优先于暴力枚举）
`MapleLib/WzLib/Util/WzTool.cs → DetectMapleVersion`
```
分别用 GMS/EMS/BMS 解析整包，统计目录+图片名字里可打印字符占比
  recognized = count(0x20<=c<=0x7E)/total   (GetDecryptionSuccessRate)
取成功率最高的区域；若最高 < 0.7 且同目录有 ZLZ.dll -> 返回 GETFROMZLZ
```

---

## 6. 暴力枚举兜底逻辑（fallback）
### 何时兜底
区域 IV 猜测 / CUSTOM / ZLZ 都拿不到可解 IV 时启动全空间搜索。

### 编排主流程 `HaRepacker/GUI/WzKeyBruteforceForm.cs → RunWzKeyBruteforce`
```
processorCount = max(1, Environment.ProcessorCount*3)
probe = new WzKeyBruteforceProbe(wzPath)      // 预载短根图片 .img 后缀约束

# A. 公共密钥短路
for c in distinct[ 0, le32(WZ_GMSIV), le32(WZ_MSEAIV) ]:
    if probe.TryCandidate(c) and publish(c): return

# B. 并行扫满 0..2^32-1
Parallel.For(0, processorCount):
    rangeStart = 2^32 * workerId      / processorCount
    rangeEnd   = 2^32 * (workerId+1)  / processorCount
    using worker = probe.CreateWorker()
    found = worker.FindFirst(rangeStart, rangeEnd, token, cancel, onProgress)
    if found and publish(found): loopState.Stop()
if 无人命中: 报 "No encryption key was found."
publish = Interlocked.CompareExchange(foundIv, candidate, -1)  首个成功者回调 UI
```

### 两级判据 `HaRepacker/GUI/WzKeyBruteforceProbe.cs` + `WzTool.TryBruteforcingWzIVKey`
```
第一级（超快批量，零分配）：
  预读短名根 .img 的后缀 -> 得到密钥流前16字节里若干 (位置=期望字节) 约束
  AES-ECB 每批 16384 个候选 IV 铺 iv[j%4] 种子块 -> 加密 -> 取16字节密钥流
  MatchesSignature: 逐约束比对，任一不符丢弃该候选
第二级（罕见，完整）：
  仅签名命中者调用 TryBruteforcingWzIVKey:
     using wzf=new WzFile(wzPath, iv)
     if wzf.ParseMainWzDirectory()!=Success: false
     return wzf.WzDirectory.WzImages[0].Name.EndsWith(".img")   // 首个图片名可解码且以.img结尾
```
判据依据：IV 错→密钥流错→名字乱码/不以 .img 结尾。

### 性能与限制（`docs/perf/wz-key-bruteforce.md`）
- 快路径要求存在"短根图片名 + 其加密 .img 后缀落在密钥流前16字节"的文件，官方推荐
  `TamingMob.wz`；不满足直接给明确错误，不做慢速全扫。
- 单 worker ~3.88 亿候选/秒，48 worker ~29.3 亿/秒；满 2^32 的签名过滤部分约 1.47 秒。
- 每 worker 2×256KiB 复用缓冲；仅用平台加速 Aes.Create()，不写 x86 SIMD（兼顾 x86/ARM64）。

---

## 7. 读取顺序建议（兜底放最后）
```
1) GetIvByMapleVersion(已知区域/CUSTOM)      能解析即用
2) DetectMapleVersion(...)                    三区域择优；<0.7且有ZLZ.dll -> GETFROMZLZ
3) ZLZ: GetIvFromZlz + GetAesKeyFromZlz
4) 全失败 -> WzKeyBruteforce（第6节）：公共短路 + 2^32 并行 + 两级判据
```

---

## 8. 落地提醒
- 若本项目（BeiDou-Client）目录内的 `BeiDou.exe` 是**编译产物**而非 C# 源码，则以上逻辑不能
  直接改到二进制；要在自己 fork 里实现，请复刻第 3、6 节算法（尤其密钥流的 `iv[j%4]` 种子 +
  密文串联、两级判据），并把第 1 节 `{13,08,06,B4,1B,0F,33,52}` 作为默认用户密钥。
- 关键在线可核对路径：
  - MapleLib 子模块源 = https://github.com/lastbattle/MapleLib
  - 主仓 = https://github.com/lastbattle/Harepacker-resurrected
  - 文件：MapleCryptoConstants.cs / WzAESConstant.cs / WzTool.cs / WzKeyGenerator.cs /
    WzMutableKey.cs / WzFile.cs；HaRepacker/GUI/WzKeyBruteforceForm.cs / WzKeyBruteforceProbe.cs；
    docs/perf/wz-key-bruteforce.md
