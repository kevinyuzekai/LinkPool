# LinkPool（网卡聚合助手）

macOS 多网卡聚合 MVP —— 受 [HypoMux](https://github.com/) 思路启发的**系统代理模式**工具：通过本地 HTTP / SOCKS5 代理，将**新的出站 TCP 连接**按权重调度到你勾选的网卡（源地址绑定）。

> **不会**拆分单条 TCP 连接，也**不是** TUN / Network Extension。适合多连接下载（多线程下载器、Steam 多连接、浏览器多请求等）。

**English summary:** LinkPool is a HypoMux-inspired macOS multi-NIC aggregation MVP. It runs a local HTTP + SOCKS5 proxy and binds each *new* outbound connection to a selected adapter via weighted smooth round-robin. Single-TCP splitting, TUN, and Windows are out of scope for v0.1.

---

## 功能（MVP）

1. 列出网卡：名称、IPv4、类型猜测；勾选 + 权重  
2. 启动 / 停止本地 HTTP + SOCKS5；状态机（stopped → running…）  
3. 可选 macOS 系统代理开关（`//go:build darwin`，完整测试需真机 Mac）  
4. 平滑加权调度新连接；每网卡流量统计  
5. 按源 IP 做出口诊断（ipify）  
6. 主机绕过 / 直连规则（精确 + 后缀）  
7. 嵌入式中文 Web 控制面板（单二进制）  
8. 单元测试：调度器、规则、bind 选择  
9. `.app` / `.dmg` 打包脚本（需在 Apple Silicon Mac 上跑 `hdiutil`）

## 架构

```
┌─────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ Browser /   │────▶│ LinkPool Proxy   │────▶│ NIC A (bind IP) │
│ Steam / …   │     │ HTTP + SOCKS5    │────▶│ NIC B (bind IP) │
└─────────────┘     │ Weighted WRR     │     └─────────────────┘
                    │ Bypass rules     │
┌─────────────┐     └────────▲─────────┘
│ Web UI :8787│──────────────┘  REST API
└─────────────┘
```

- **Core (Go)**：`internal/proxy`、`scheduler`、`adapter`、`rules`、`sysproxy`  
- **UI**：嵌入 `web/static` 的单页控制台（非 Wails；优先可维护与可打包）  
- **系统代理**：`networksetup`（仅 darwin）

### 与 HypoMux 的差异 / 局限

| | LinkPool v0.1 | 典型 HypoMux 类产品 |
|--|--|--|
| 模式 | 本地代理 + 可选系统代理 | 可能含更完整桌面壳 / 优化器 |
| 单 TCP 拆分 | ❌ | 通常也不做（多连接调度） |
| TUN / NE | ❌ 明确不做 | 部分产品有 |
| 进程规则 | ❌ | 可能有 |
| Windows | ❌ | 可能有 |
| 打包 | `.app` + `hdiutil` DMG 脚本 | 正式公证 / Sparkle 等 |

## 在 Apple Silicon Mac 上构建

需要：Go 1.22+、Xcode CLT（`networksetup` / 可选 `iconutil`）。

```bash
git clone https://github.com/kevinyuzekai/LinkPool.git
cd LinkPool
make test
make build          # → bin/linkpool
./bin/linkpool -open
```

### 打出 .app 与 .dmg（必须在 macOS 上）

```bash
./scripts/build-macos-arm64.sh   # 生成 build/macos/LinkPool.app + 裸二进制
./scripts/package-dmg.sh         # 生成 build/macos/LinkPool-0.1.0-arm64.dmg
```

可选图标：

```bash
# 需 rsvg-convert + iconutil
./scripts/make-icns.sh
./scripts/build-macos-arm64.sh
```

> **Linux / CI**：可交叉编译 `GOOS=darwin GOARCH=arm64` 的二进制与 `.app` 目录布局，但**无法**在本机生成真正的 `.dmg`（没有 `hdiutil`）。请把仓库拉到 MacBook Pro 上执行 `package-dmg.sh`。

交叉编译示例：

```bash
make build-darwin-arm64
# 或
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o build/macos/LinkPool ./cmd/linkpool
```

## 使用方法

1. 启动 LinkPool，浏览器打开 `http://127.0.0.1:8787`  
2. 刷新网卡，勾选要用的适配器并设置权重  
3. 点击「启动」；可选勾选「启用 macOS 系统代理」  
4. 或手动配置客户端：

### 浏览器

- HTTP 代理：`127.0.0.1:18080`  
- SOCKS5：`127.0.0.1:11080`

### Steam

Steam → 设置 → 下载 / 网络（或「互联网」）→ 使用 HTTP / SOCKS 代理，填入同上。  
多连接下载时，新连接会按权重落到不同网卡。

### curl 自测

```bash
curl -x http://127.0.0.1:18080 https://api.ipify.org
curl --socks5 127.0.0.1:11080 https://api.ipify.org
```

## Linux 上能测什么？

| 能力 | Linux | macOS |
|--|--|--|
| 单元测试（调度 / 规则 / bind） | ✅ | ✅ |
| 网卡枚举（`net.Interfaces`） | ✅ 有限 | ✅ + `networksetup` 友好名 |
| HTTP / SOCKS5 代理 + 源绑定 | ✅（路由允许时） | ✅ |
| 系统代理 set/restore | ❌ stub | ✅ |
| `.dmg` | ❌ | ✅ `hdiutil` |
| 完整多网卡体验 | 受限 | ✅ 真机 |

## 开发

```bash
make tidy
make test
make run
```

默认端口：

- UI：`127.0.0.1:8787`  
- HTTP 代理：`127.0.0.1:18080`  
- SOCKS5：`127.0.0.1:11080`

## 目录结构

```
cmd/linkpool/          入口
internal/adapter/      网卡发现（darwin / linux）
internal/scheduler/    平滑加权轮询
internal/proxy/        HTTP + SOCKS5 + bind dialer
internal/rules/        绕过规则
internal/sysproxy/     macOS 系统代理
internal/stats|diag|api|app/
web/static/            嵌入式 UI
packaging/macos/       Info.plist、图标源
scripts/               build-macos-arm64.sh、package-dmg.sh
```

## 许可

MIT © 2026 kevinyuzekai

---

## English — quick start on Mac

```bash
git clone https://github.com/kevinyuzekai/LinkPool.git && cd LinkPool
make test && make build && ./bin/linkpool -open
# Package:
./scripts/build-macos-arm64.sh && ./scripts/package-dmg.sh
```

Point Steam/browser at `127.0.0.1:18080` (HTTP) or `:11080` (SOCKS5). DMG packaging **requires** an Apple Silicon Mac (`hdiutil`). This repo ships packaging scripts and a runnable single binary with embedded Chinese UI.
