# LinkPool（网卡聚合助手）

macOS 多网卡聚合工具 —— 受 HypoMux 思路启发的**系统代理模式**：通过本地 HTTP / SOCKS5 代理，将**新的出站 TCP 连接**调度到你勾选的网卡（源地址绑定），并尽量让多网卡带宽**叠加**。

> **不会**拆分单条 TCP，也**不是** TUN / Network Extension / MPTCP。  
> 多连接客户端（Steam、多线程下载器）会自动叠加；**单文件**可用内置「分段加速下载」（HTTP Range 多段并行，每段绑不同网卡）尽量接近完全叠加。

**English summary:** LinkPool is a HypoMux-inspired macOS multi-NIC aggregation tool. It runs a local HTTP + SOCKS5 proxy, binds each *new* outbound connection to a selected adapter (adaptive weighted WRR + dial failover), and includes an optional HTTP multi-range downloader so a single file can stack NIC bandwidth. Single-TCP bonding and TUN remain out of scope.

**Current version: 0.2.0**

---

## v0.2.0 速度相关改进

1. **自适应权重（默认，≥2 网卡时生效）**  
   按近期吞吐（EMA）提高更快网卡的有效权重；慢速 / 高错误网卡降权（最低 1）。可切换回「固定权重」。
2. **拨号失败自动换卡**  
   `dialBound` 失败时依次尝试其他网卡；连续失败会短暂降权 / 跳过。
3. **软最少连接**  
   权重接近时略微偏向活跃连接更少的网卡。
4. **分段加速下载**  
   控制面板可将一个 HTTP(S) URL 拆成 N 段并行 Range 请求（默认 N = 已选网卡数 × 2），每段绑定不同网卡后拼成完整文件 —— 这是代理模式下单文件接近「完全叠加」的实用做法。
5. **UI**  
   每网卡近似速率（KB/s · MB/s）、调度模式切换、叠加说明提示。
6. **双架构 macOS 打包**  
   同时支持 Apple Silicon（arm64）与 Intel（amd64 / x86_64）的 `.app` / `.dmg` 脚本。

### 局限（请知悉）

| 能力 | 状态 |
|--|--|
| 多连接下载叠加 | ✅ 代理调度 |
| 单文件 HTTP Range 叠加 | ✅ 内置分段下载 |
| 单条 TCP 拆分 / bonding | ❌ 物理/协议限制 |
| TUN / Network Extension / MPTCP | ❌ 本版不做 |
| 进程级规则 | ❌ |
| Windows | ❌ |

---

## 功能一览

1. 列出网卡：名称、IPv4、类型；勾选 + 权重 + 近似速率  
2. 启动 / 停止本地 HTTP + SOCKS5  
3. 可选 macOS 系统代理（`darwin`）  
4. 平滑加权 / 自适应调度；拨号 failover  
5. 源 IP 出口诊断（ipify）  
6. 主机绕过规则  
7. 嵌入式中文 Web 控制面板 + 分段加速下载  
8. 单元测试：调度器、自适应、failover、规则、bind  
9. `.app` / `.dmg` 打包脚本（**arm64 + amd64**；DMG 需在 Mac 上跑 `hdiutil`）

## 架构

```
┌─────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ Browser /   │────▶│ LinkPool Proxy   │────▶│ NIC A (bind IP) │
│ Steam / …   │     │ HTTP + SOCKS5    │────▶│ NIC B (bind IP) │
└─────────────┘     │ Adaptive WRR     │     └─────────────────┘
                    │ Dial failover    │
┌─────────────┐     │ Multi-Range DL   │
│ Web UI :8787│────▶│                  │
└─────────────┘     └──────────────────┘
```

## 下载哪个版本？（arm64 vs Intel）

| 你的 Mac | 芯片 | 下载 / 构建产物 |
|--|--|--|
| 2020 年底及以后多数机型 | Apple Silicon（M1/M2/M3/…） | `LinkPool-0.2.0-arm64.dmg` |
| 较旧的 Intel Mac | Intel x86_64 | `LinkPool-0.2.0-amd64.dmg` |

不确定时可点左上角 →「关于本机」：若显示「芯片」为 Apple M* 选 arm64；若显示「处理器」为 Intel 选 amd64。

## 在 Mac 上构建

需要：Go 1.22+、Xcode CLT（`networksetup` / 可选 `iconutil`）。

```bash
git clone https://github.com/kevinyuzekai/LinkPool.git
cd LinkPool
make test
make build          # → bin/linkpool
./bin/linkpool -open
```

### 打出 .app 与 .dmg（必须在 macOS 上打 DMG）

```bash
# Apple Silicon
ARCH=arm64 ./scripts/build-macos.sh          # 或 ./scripts/build-macos-arm64.sh
ARCH=arm64 ./scripts/package-dmg.sh          # → build/macos/arm64/LinkPool-0.2.0-arm64.dmg

# Intel Mac
ARCH=amd64 ./scripts/build-macos.sh          # 或 ./scripts/build-macos-amd64.sh
ARCH=amd64 ./scripts/package-dmg.sh          # → build/macos/amd64/LinkPool-0.2.0-amd64.dmg
```

脚本会在 Darwin 上对 `.app` 做 **ad-hoc codesign**。  
首次打开若被 Gatekeeper 拦截：系统设置 → 隐私与安全性 → 仍要打开，或执行 `xattr -cr /path/to/LinkPool.app`。  
**无原生窗口** — 启动后用浏览器打开 `http://127.0.0.1:8787`。

> **Linux / CI**：可交叉编译 arm64 **与** amd64 的二进制 / `.app` 目录布局，并跑单元测试；**无法**在本机生成真正的 `.dmg`（没有 `hdiutil`）。请把仓库拉到对应架构的 Mac 上执行 `package-dmg.sh`。

Makefile 快捷方式：

```bash
make build-darwin-arm64   # ARCH=arm64
make build-darwin-amd64   # ARCH=amd64
# 在 Mac 上：
make package-dmg          # arm64 DMG
make package-dmg-amd64    # amd64 DMG
```

## 使用方法（叠加带宽）

1. 启动 LinkPool，打开 `http://127.0.0.1:8787`  
2. 勾选 **≥2** 张网卡；调度保持「自适应加速」  
3. 点击「启动」（可选系统代理）  
4. **多连接场景**（推荐）：Steam / IDM / aria2 等多连接走代理 → 新连接自动分到各网卡  
5. **单文件 HTTP**：在「分段加速下载」粘贴 URL → 开始 → 完成后保存文件  

### 浏览器 / Steam 代理

- HTTP：`127.0.0.1:18080`  
- SOCKS5：`127.0.0.1:11080`

### curl 自测

```bash
curl -x http://127.0.0.1:18080 https://api.ipify.org
curl --socks5 127.0.0.1:11080 https://api.ipify.org
```

### 分段下载 API

```bash
curl -s -X POST http://127.0.0.1:8787/api/download \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com/file.bin","concurrency":0}'
# 轮询 GET /api/download/{id} ，完成后 GET /api/download/{id}/file
```

## Linux 上能测什么？

| 能力 | Linux | macOS |
|--|--|--|
| 单元测试 | ✅ | ✅ |
| 交叉编译 darwin/arm64 + amd64 | ✅ | ✅ |
| 代理 + 源绑定 | ✅（路由允许时） | ✅ |
| 分段下载 | ✅ | ✅ |
| 系统代理 | ❌ stub | ✅ |
| `.dmg` | ❌ | ✅ `hdiutil` |

## 开发

```bash
make tidy && make test && make run
```

默认端口：UI `127.0.0.1:8787` · HTTP `18080` · SOCKS5 `11080`

## 目录结构

```
cmd/linkpool/          入口
internal/adapter/      网卡发现
internal/scheduler/    平滑 WRR + 自适应权重 + 降权
internal/proxy/        HTTP + SOCKS5 + bind dialer（failover）
internal/download/     HTTP(S) 多 Range 分段下载
internal/rules|stats|diag|api|app|sysproxy/
web/static/            嵌入式 UI
packaging/macos/       Info.plist、图标源
scripts/               build-macos.sh（ARCH=arm64|amd64）、package-dmg.sh
```

## 许可

MIT © 2026 kevinyuzekai

---

## English — quick start

```bash
git clone https://github.com/kevinyuzekai/LinkPool.git && cd LinkPool
make test && make build && ./bin/linkpool -open

# Apple Silicon Mac:
ARCH=arm64 ./scripts/build-macos.sh && ARCH=arm64 ./scripts/package-dmg.sh
# Intel Mac:
ARCH=amd64 ./scripts/build-macos.sh && ARCH=amd64 ./scripts/package-dmg.sh
```

Select ≥2 NICs, keep **adaptive** scheduling, start proxy. Use multi-connection downloaders or the built-in **multi-range** tool for single-file stacking. Download **arm64** DMG for Apple Silicon, **amd64** for Intel. Single-TCP bonding / TUN are not in this release.
