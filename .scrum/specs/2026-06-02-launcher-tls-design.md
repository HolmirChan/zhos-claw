# Launcher TLS 支持 + 自签名证书自动签发

> 创建: 2026-06-02
> 来源: BACKLOG.md (BL-010)
> 状态: 设计完成

## 背景

RK3506 设备通过 HTTP + IP 地址访问时，浏览器的安全上下文策略阻止 `getUserMedia()` 和 `AudioWorklet`，导致语音录音功能静默失败。HTTPS 访问（即使是自签名证书）可满足安全上下文要求。

## 设计目标

1. Launcher 启动即自动启用 HTTPS，零配置零依赖
2. 自签名证书在 Go 代码内生成，不依赖外部 openssl
3. 设备 IP 变化时自动重新签发证书
4. 双端口并存，不影响现有 HTTP 流程
5. 现有部署流程无需额外手动步骤

## 架构

```
Launcher 启动
    │
    ├── -public 开启？
    │   ├── 否 → 跳过 HTTPS（localhost 已是 secure context）
    │   └── 是 → 继续
    │
    ├── 校验 tlsPort != httpPort（冲突直接 fatal）
    │   ↑ 仅在 HTTPS 确实要启动时校验，避免不开 -public 的本地开发者被误杀
    │
    ├── 检查 <PICOCLAW_HOME>/tls/meta.json 是否存在
    │   ├── 无 → 生成新证书
    │   └── 有 → schema_version 匹配？
    │       ├── 否 → 重新生成证书
    │       └── 是 → 且 meta.clock_fallback 且当前时钟 ≥ 2020？
    │           ├── 是 → 重新生成证书（NTP 同步后用正常证书替换 fallback）
    │           └── 否 → meta.sans ⊇ 当前所有合规 LAN IP（已过滤 link-local）？
    │               ├── 是 → 加载缓存证书
    │               └── 否 → 重新生成证书
    │
    ├── HTTP Server:  netbind.OpenPlan(plan, "18800")  (原有，不动)
    └── HTTPS Server: netbind.OpenPlan(plan, "18443")  (新增，仅 -public)
        │
        └── 每个 listener → srv.ServeTLS(ln, "", "")
           ↑ srv.TLSConfig 已设 Certificates + NextProtos
           ↑ srv 加入 servers 切片，参与 Graceful Shutdown
```

### 关键决策

- **自签名、非 CA 签发**：浏览器首次访问弹警告，用户点"继续访问"后浏览器记住例外，后续不再弹。证书 SAN 集合不变则例外持续有效
- **仅 `-public` 时启用 HTTPS**：localhost 本身就是 secure context，无需 TLS；只有局域网访问才需要
- **自动生成、自动过期**：正常路径 365 天过期，fallback 路径 +10 年（NTP 同步后自动替换）
- **ECDSA P256**：性能优于 RSA，Go `crypto/ecdsa` 标准库直接支持
- **SAN 仅绑定 IPv4 LAN + ::1**：IPv6 GUA/ULA 在 DHCPv6 PD 下会变导致反复重签，不纳入
- **遵循 -public 标志**：HTTPS 和 HTTP 绑定策略一致（netbind.Plan 同一实例）
- **ServeTLS 标准路径**：使用 `srv.ServeTLS(ln, "", "")`，内部自动启用 HTTP/2 + ALPN

## 新增 Flag

```
-no-tls          显式关闭 HTTPS（即使 -public 开启）
-tls-port int    HTTPS 端口（默认 18443）
```

HTTPS 默认行为：`-public` 开启时默认启用，`-public` 关闭时默认跳过（localhost 无需 TLS）。

启动时立即校验 `tlsPort != httpPort`，冲突直接 `logger.Fatal` 并提示原因。

不提供 `-tls-cert` / `-tls-key` flag，证书自动生成，无需用户指定。

## 证书自动生成

### 生成算法

```go
func generateSelfSignedCert(ips []net.IP) (certPEM, keyPEM []byte, clockFallback bool, err error) {
    // 1. 生成 ECDSA P256 私钥
    key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
    if err != nil {
        return nil, nil, false, fmt.Errorf("generate key: %w", err)
    }

    // 2. SerialNumber: 128-bit 随机数（RFC 5280）
    serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
    if err != nil {
        return nil, nil, false, fmt.Errorf("generate serial: %w", err)
    }

    // 3. 处理嵌入式设备 RTC 未同步
    now := time.Now()
    clockFallback = false
    var notBefore time.Time
    if now.Before(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) {
        logger.Warn("系统时钟未同步，证书将使用后备时间范围（NotAfter +10 年）")
        notBefore = time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)
        clockFallback = true
    } else {
        notBefore = now.Add(-24 * time.Hour)
    }

    // 4. SAN: 所有合规 LAN IPv4 + 127.0.0.1 + ::1 + DNSNames: ["localhost"]
    //    过滤规则：排除 IPv6 link-local（fe80::/10）、IPv4 link-local（169.254/16）
    //    IPv6 GUA/ULA 不纳入 SAN（DHCPv6 PD 变化会反复触发重签）
    sanIPs := append([]net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}, ips...)

    // 5. 构造 X.509 模板
    notAfter := now.Add(365 * 24 * time.Hour)
    if clockFallback {
        notAfter = time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)
    }

    template := &x509.Certificate{
        SerialNumber:            serial,
        Subject:                 pkix.Name{CommonName: "ZhosClaw TLS"},
        NotBefore:               notBefore,
        NotAfter:                notAfter,
        KeyUsage:                x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
        ExtKeyUsage:             []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
        BasicConstraintsValid:   true,
        IsCA:                    false,
        IPAddresses:             sanIPs,
        DNSNames:                []string{"localhost"},
    }

    // 6. 自签名
    certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
    if err != nil {
        return nil, nil, false, fmt.Errorf("create certificate: %w", err)
    }

    // 7. PEM 编码
    certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
    keyBytes, err := x509.MarshalECPrivateKey(key)
    if err != nil {
        return nil, nil, false, fmt.Errorf("marshal key: %w", err)
    }
    keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

    return certPEM, keyPEM, clockFallback, nil
}
```

所有 err 必须 propagate，生成失败 → `logger.Error` + HTTPS 不启动，HTTP 不受影响。

### getAllLocalIPs 过滤规则

```
包含：IPv4 LAN（非 127.x、非 169.254/16、非 0.0.0.0）
排除：IPv6 link-local（fe80::/10）
排除：IPv4 link-local（169.254/16）
排除：IPv6 GUA（global unicast, 2000::/3）
排除：IPv6 ULA（fc00::/7）
包含（固定）：127.0.0.1、::1
```

### 缓存结构

```
<PICOCLAW_HOME>/tls/
├── server.crt    # PEM 编码的 X.509 证书
├── server.key    # PEM 编码的 ECDSA 私钥
└── meta.json     # {
                  #   "schema_version": 1,
                  #   "sans": ["127.0.0.1","::1","192.168.18.38"],
                  #   "clock_fallback": false,
                  #   "generated_at": "2026-06-02T10:00:00Z"
                  # }
```

`schema_version`: 将来扩展 SAN 列表或改算法时强制 invalidate 旧缓存。
`clock_fallback`: 记录生成时是否使用了 fallback 时钟。NTP 同步后下次启动检测到此字段为 true 且 `time.Now() >= 2020`，强制重生成。

### IP 匹配逻辑（集合包含关系）

```go
func (m *tlsMeta) matchesCurrentIPs() bool {
    currentIPs := getAllLocalIPs()
    cachedSet := make(map[string]struct{}, len(m.SANs))
    for _, san := range m.SANs {
        cachedSet[san] = struct{}{}
    }
    for _, ip := range currentIPs {
        if _, ok := cachedSet[ip]; !ok {
            return false // 出现了缓存中没有的新 IP
        }
    }
    return true // 当前 IP 集合 ⊆ 缓存 SAN 集合
}
```

不关心"缓存多了 IP"（删了张网卡无所谓），只关心"多出新 IP"（SAN 覆盖不全要重签）。

### 文件写入原子性 + 并发保护

使用平台特定的文件锁（`syscall.Flock` on Unix, `LockFileEx` on Windows），拆分为：
- `web/backend/tls_lock_unix.go`（build tag `!windows`）
- `web/backend/tls_lock_windows.go`（build tag `windows`）

```go
func lockTLS(dir string, lockPath string) (func(), error) {
    f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
    if err != nil {
        return nil, err
    }
    if err := flock(f); err != nil { // 跨平台抽象
        f.Close()
        return nil, err
    }
    return func() { flockUnlock(f); f.Close() }, nil
}
```

三个文件先写 `.tmp` 后缀，再逐个 `os.Rename`。进程崩溃时 flock 由内核自动释放，不会永久锁死。

## HTTPS Server 启动

使用官方推荐 `srv.ServeTLS` 路径（内置 HTTP/2 + ALPN 初始化）：

```go
cert, err := tls.X509KeyPair(certPEM, keyPEM) // PEM → tls.Certificate
if err != nil {
    return fmt.Errorf("parse key pair: %w", err)
}
tlsCfg := &tls.Config{
    Certificates: []tls.Certificate{cert},
    MinVersion:   tls.VersionTLS12,
}
srv := &http.Server{
    Handler:   handler,
    TLSConfig: tlsCfg,
}
// http2.ConfigureServer 由 ServeTLS 自动调用，无需手动
srv.ServeTLS(ln, "", "") // 空路径 → 从 TLSConfig.Certificates 取
```

## HTTPS 地址暴露

### `/api/system/version` 响应新增字段

```json
{
  "version": "...",
  "build_info": {...},
  "http_url": "http://192.168.18.38:18800",
  "https_url": "https://192.168.18.38:18443"
}
```

`https_url` 在 `-public=false` 或 `-no-tls` 时为空字符串。前端 tooltip 从此读取端口，不硬编码 18443。

向后兼容：仅新增 `http_url` / `https_url` 字段，旧字段保留原 schema 不变。

## Launcher 改动

### 文件：`web/backend/main.go`

1. 新增 `-no-tls` 和 `-tls-port` flag 解析
2. 启动后立即校验 `tlsPort != httpPort`，冲突 `logger.Fatal`
3. 新增 `ensureTLS(plan netbind.Plan, port, home string) (netbind.OpenResult, error)` 函数：
   - 返回 `netbind.OpenResult`（含 `[]net.Listener`），与 HTTP 同款结构
   - 检查缓存 → 生成/加载证书 → `netbind.OpenPlan(plan, port)` → 每个 listener 走 `srv.ServeTLS(ln, "", "")`
   - 仅在 `-public` 开启且未设 `-no-tls` 时调用；`-public=false` 直接跳过
   - 端口冲突时 `netbind.OpenPlan` 返回明确错误，不静默失败
4. HTTPS `*http.Server` 加入 `servers` 切片，与 HTTP 一同参与 `shutdownApp` Graceful Shutdown
5. 启动日志同时打印 HTTP 和 HTTPS 地址
6. 控制台 `consoleHosts` 打印也加 HTTPS 地址
7. Launcher 重启时（systray / config 变更触发）保持同样逻辑

### -public 与 HTTPS 行为

| -public | -no-tls | HTTP (18800) | HTTPS (18443) |
|---------|---------|-------------|---------------|
| false | — | loopback only | **不启动**（localhost 已是 secure context） |
| true | false | 0.0.0.0 | 0.0.0.0（默认启用） |
| true | true | 0.0.0.0 | **不启动**（显式关闭） |

两者使用同一个 `netbind.Plan`。

## 前端改动

### 文件：`web/frontend/src/components/chat/voice-recorder.tsx`

VoiceRecorder 自管理 disabled 状态：

```tsx
const isSecure = typeof window !== "undefined" && window.isSecureContext;
const isSupported = typeof AudioContext !== "undefined" && typeof WebSocket !== "undefined";

if (!isSupported) return null;

if (!isSecure) {
  // 从 /api/version 拉 https_url，或 catch 后用默认端口 18443
  return (
    <button disabled title={`语音录音需通过 HTTPS 访问。请访问 ${httpsUrl}`}>
      <MicOffIcon />
      <span>需 HTTPS</span>
    </button>
  );
}

// 安全上下文 → 正常渲染录音按钮
```

### 文件：`web/frontend/src/components/chat/chat-composer.tsx`

确认现状后决定：如果 chat-composer 当前没有 `isSecureContext` 判断，无需改；如果有，移除重复判断，改由 VoiceRecorder 自管理。

实现前 `grep -n "isSecure\|secureContext" web/frontend/src/components/chat/chat-composer.tsx` 确认。

### 文件：`web/frontend/src/store/voice.ts`

新增 `httpsUrlAtom`（从 `/api/system/version` 读取）。

### 文件：`web/frontend/src/api/voice.ts`

`fetchVoiceCapabilities` 或新增独立函数拉取 `/api/system/version`，提取 `https_url` 字段。

### Mixed-Content 审计

实现阶段全量 `grep http://` 审计前端和模板文件，站内自引用改为协议相对路径（`//`）或相对路径。审计范围：`web/frontend/src/**`；`dist/` 是构建产物无需单独检查。

## Dashboard 状态面板

Launcher 的「关于/状态」面板显式列出两个入口（从 `/api/system/version` 读取 `http_url` / `https_url`）。

## 部署脚本改动

### 文件：`scripts/deploy-rk3506.sh`

init 脚本启动命令不变（`-public` 自动启用 TLS，无额外参数）：

```sh
nohup ${REMOTE_DIR}/${CMD}-web-linux-arm -public >> ${LOG_DIR}/launcher.log 2>&1 &
```

部署完成提示：

```diff
- echo "部署完成！访问 http://${DEVICE_IP}:18800"
+ echo "部署完成！"
+ echo "  HTTP:  http://${DEVICE_IP}:18800"
+ echo "  HTTPS: https://${DEVICE_IP}:18443  (语音功能)"
```

设备上 `tls/` 缓存目录由 launcher 自动创建，无需脚本手动处理。

## 边界场景

| 场景 | 行为 |
|------|------|
| 首次启动（-public） | 自动生成证书，<10ms |
| -public=false | 不启动 HTTPS，不生成证书 |
| -no-tls | 不启动 HTTPS，不生成证书 |
| -tls-port == -port | 启动直接 fatal，提示端口冲突 |
| 设备新增 LAN IPv4 | SAN 集合不包含新 IP → 重新生成 |
| 设备删除 IP | 不重生成（当前 ⊆ 缓存 SAN） |
| link-local IP 变化 | 不影响（已过滤） |
| IPv6 GUA/ULA 变化 | 不影响（不纳入 SAN） |
| 时钟未同步（<2020） | fallback 基准 2019，NotAfter = 2099-12-31，meta.clock_fallback=true |
| NTP 同步后重启 | meta.clock_fallback=true + now >= 2020 → 强制重生成正常证书 |
| NTP 同步后不重启 | fallback 证书继续生效（NotAfter=2099，功能不受影响）；下次重启自动替换（by-design，不做后台 goroutine 检查） |
| 证书过期（365 天） | 到期时自动重新生成 |
| 缓存目录不可写 | logger.Error，HTTPS 不启动，HTTP 正常 |
| 18443 端口被占用 | netbind.OpenPlan 返回错误，logger.Error |
| 并发写入 tls/ | flock + .tmp + rename |
| schema_version 不匹配 | 强制重新生成证书 |
| 优雅关闭 (SIGTERM) | HTTPS server 加入 servers 切片，同等 Graceful Shutdown |
| Windows 平台 | tls_lock_windows.go 使用 LockFileEx |

## 测试覆盖

### 单测（`web/backend/tls_test.go`，新建）

实现时把判定逻辑抽到独立纯函数以便单测，main() 只负责编排：
- `shouldStartTLS(public, noTLS bool) bool`
- `validateTLSPort(httpPort, tlsPort int) error`
- `generateSelfSignedCert(ips []net.IP) (certPEM, keyPEM []byte, clockFallback bool, err error)`

| 测试函数 | 覆盖场景 |
|---------|---------|
| `TestGenerateSelfSignedCert` | 空 IP 列表、单 IP、多 IP、返回有效 PEM、NotAfter > NotBefore |
| `TestGenerateCertErrorPropagation` | mock rand.Reader 失败，断言 err != nil |
| `TestSanFiltering` | fe80:: link-local 被排除、169.254.x.x 被排除、192.168.x LAN 保留、IPv6 GUA 被排除 |
| `TestMatchesCurrentIPs` | 包含（true）、新增 IP（false）、删除 IP（true）、空缓存、schema_version 不匹配（false） |
| `TestClockFallback` | now < 2020 → clockFallback=true + NotAfter ≥ 2099-12-31 |
| `TestClockFallbackPersisted` | mock 2000 生成 → meta.clock_fallback=true；mock 2026 加载 → 触发重生成 |
| `TestFileWriteAtomic` | 模拟中途失败（写入第一个 .tmp 后 panic），断言下次加载能清理 .tmp 并恢复 |
| `TestConcurrentEnsure` | N 个 goroutine 同时调用 ensure，断言只生成一次（文件 mtime 不变） |
| `TestHTTPSNotStartedWithoutPublic` | -public=false → TLS listener 列表为空 |
| `TestHTTPSNotStartedWhenDisabled` | -no-tls → TLS listener 列表为空 |
| `TestTLSPortEqualsHTTPPort` | -tls-port=18800 → 启动直接 fatal |
| `TestWindowsLockFallback` | 如果 CI 跑 Windows：验证 tls_lock_windows.go 行为正确 |

验收跑 `go test -v -tags goolm,stdjson ./web/backend/...` 全部 PASS。

## 验收标准

- [ ] `-public` 开启时自动生成自签名证书，缓存到 `<home>/tls/`
- [ ] `-public=false` 时不启动 HTTPS，不生成证书
- [ ] `-no-tls` 显式关闭 HTTPS
- [ ] `-tls-port == -port` 启动直接 fatal
- [ ] HTTPS 端口 18443（可通过 `-tls-port` 修改）
- [ ] HTTP 和 HTTPS 共用 netbind.Plan，双端口行为一致
- [ ] 设备 IP 集合未变时，启动 5 次仍使用同一证书（mtime 不变）
- [ ] 设备新增 LAN IPv4 时证书自动重新生成
- [ ] link-local / IPv6 GUA/ULA 变化不触发重生成
- [ ] 时钟未同步（RTC < 2020）时生成 NotAfter ≥ 2099 的 fallback 证书，meta.clock_fallback=true
- [ ] NTP 同步后重启，检测 clock_fallback → 自动替换为正常证书
- [ ] 多网卡设备从任一 LAN IP 访问 HTTPS 不报 hostname mismatch
- [ ] 18443 被占用时打印明确错误，不静默失败
- [ ] HTTPS server 参与 graceful shutdown
- [ ] `/api/system/version` 包含 `https_url` 字段
- [ ] 前端非安全上下文显示 disabled 麦克风 + 从 `/api/system/version` 读取端口显示 tooltip
- [ ] 前端无硬编码 `http://` 自引用导致 mixed-content
- [ ] 浏览器访问 `https://IP:18443` 录音功能正常工作
- [ ] 控制台和文件日志同时打印 HTTPS 入口地址
- [ ] 部署脚本无需额外证书推送步骤
- [ ] `make build-launcher` 编译通过
- [ ] `go test -tags goolm,stdjson ./web/backend/...` 全部 PASS（含新增 tls 单测）