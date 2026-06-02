# Launcher TLS + 自签名证书 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Launcher 启动时自动生成自签名证书并启用 HTTPS 双端口（HTTP 18800 + HTTPS 18443），解决设备 HTTP 非 localhost 下语音录音不可用问题。

**Architecture:** 新增 `web/backend/tls.go` 负责证书生成/缓存/校验。`ensureTLS` 内部调用 `netbind.BuildPlan` + `netbind.OpenPlan` 绑定 HTTPS 端口，`srv.ServeTLS` 启动。仅 `-public` 时启用。前端 chat-composer 检查 `isSecureContext`，非安全上下文 disabled 麦克风按钮 + tooltip。

**Tech Stack:** Go `crypto/ecdsa` / `crypto/x509` / `crypto/tls`（标准库），React/TypeScript（jotai state）

**实施注意:** Go 单文件只允许一个 `import` 块。以 Task 1 Step 3 的 import 为基础，后续 Task 仅向已有 import 块追加必要的包，不要新建独立 import 块。

---

## 文件结构

| 操作 | 文件 | 职责 |
|------|------|------|
| 新建 | `web/backend/tls.go` | 证书生成、缓存管理、IP 过滤、ensureTLS 编排 |
| 新建 | `web/backend/tls_test.go` | 全部单测 |
| 新建 | `web/backend/tls_lock_unix.go` | Unix flock |
| 新建 | `web/backend/tls_lock_windows.go` | Windows LockFileEx stub |
| 修改 | `web/backend/main.go` | flag 解析、HTTPS server 启动、shutdown、控制台打印 |
| 修改 | `web/backend/api/version.go` | `systemVersionResponse` 新增 `http_url` / `https_url` |
| 修改 | `web/backend/api/router.go` | `Handler` 新增 `httpURL` / `httpsURL` 字段 + setter |
| 修改 | `web/frontend/src/components/chat/chat-composer.tsx` | 🎤 按钮 isSecure 检测 |
| 修改 | `web/frontend/src/api/voice.ts` | 新增 `fetchVersionURLs()` |
| 修改 | `scripts/deploy-rk3506.sh` | 打印 HTTPS 地址 |

---

### Task 1: TLS 核心 — 判定逻辑 + IP 过滤纯函数

**Files:**
- Create: `web/backend/tls.go`
- Create: `web/backend/tls_test.go`

- [ ] **Step 1: 写单测**

`web/backend/tls_test.go`:

```go
package main

import (
	"net"
	"testing"
)

func TestShouldStartTLS(t *testing.T) {
	tests := []struct {
		name   string
		public bool
		noTLS  bool
		want   bool
	}{
		{"public without notls", true, false, true},
		{"public with notls", true, true, false},
		{"not public without notls", false, false, false},
		{"not public with notls", false, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldStartTLS(tt.public, tt.noTLS); got != tt.want {
				t.Errorf("shouldStartTLS(%v, %v) = %v, want %v", tt.public, tt.noTLS, got, tt.want)
			}
		})
	}
}

func TestValidateTLSPort(t *testing.T) {
	if err := validateTLSPort(18800, 18443); err != nil {
		t.Errorf("different ports should be ok: %v", err)
	}
	if err := validateTLSPort(18800, 18800); err == nil {
		t.Error("same ports should error")
	}
}

func TestAcceptIP(t *testing.T) {
	// LAN IPv4 → accept
	if !acceptIP(net.ParseIP("192.168.1.1")) {
		t.Error("192.168.1.1 should be accepted")
	}
	// Loopback → rejected by caller logic (caller hardcodes 127.0.0.1)
	// Link-local IPv4 → reject
	if acceptIP(net.ParseIP("169.254.1.1")) {
		t.Error("169.254.1.1 link-local should be rejected")
	}
	// Link-local IPv6 → reject
	if acceptIP(net.ParseIP("fe80::1")) {
		t.Error("fe80::1 link-local should be rejected")
	}
	// IPv6 GUA → reject
	if acceptIP(net.ParseIP("2001:db8::1")) {
		t.Error("2001:db8::1 GUA should be rejected")
	}
	// IPv6 ULA → reject
	if acceptIP(net.ParseIP("fd00::1")) {
		t.Error("fd00::1 ULA should be rejected")
	}
	// 0.0.0.0 → reject
	if acceptIP(net.ParseIP("0.0.0.0")) {
		t.Error("0.0.0.0 should be rejected")
	}
}

func TestGetAllLocalIPs(t *testing.T) {
	ips := getAllLocalIPs()
	foundV4Loopback := false
	foundV6Loopback := false
	for _, ip := range ips {
		if ip.Equal(net.IPv4(127, 0, 0, 1)) {
			foundV4Loopback = true
		}
		if ip.Equal(net.IPv6loopback) {
			foundV6Loopback = true
		}
		// No link-local should appear
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			t.Errorf("link-local IP must not appear in result: %s", ip)
		}
	}
	if !foundV4Loopback {
		t.Error("127.0.0.1 must always be present")
	}
	if !foundV6Loopback {
		t.Error("::1 must always be present")
	}
}
```

- [ ] **Step 2: 跑单测确认 FAIL**

Run: `go test -v -tags goolm,stdjson -run "TestShouldStartTLS|TestValidateTLSPort|TestAcceptIP|TestGetAllLocalIPs" ./web/backend/`
Expected: FAIL (undefined functions)

- [ ] **Step 3: 实现纯函数**

`web/backend/tls.go`:

```go
package main

import (
	"fmt"
	"net"
)

func shouldStartTLS(public, noTLS bool) bool {
	if !public {
		return false
	}
	return !noTLS
}

func validateTLSPort(httpPort, tlsPort int) error {
	if httpPort == tlsPort {
		return fmt.Errorf("HTTPS 端口 (%d) 不能与 HTTP 端口 (%d) 相同", tlsPort, httpPort)
	}
	return nil
}

func acceptIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return false
	}
	if ip.To4() != nil {
		return true // IPv4 LAN
	}
	// IPv6: only keep nothing beyond loopback (::1 added separately)
	// GUA (2000::/3), ULA (fc00::/7), and everything else → reject
	return false
}

func getAllLocalIPs() []net.IP {
	ips := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}

	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP
			if acceptIP(ip) {
				if ip.To4() != nil {
					ips = append(ips, ip.To4())
				}
				// IPv6: only ::1 was already added at the top; no other IPv6 accepted
			}
		}
	}
	return ips
}
```

- [ ] **Step 4: 跑单测确认 PASS**

Run: `go test -v -tags goolm,stdjson -run "TestShouldStartTLS|TestValidateTLSPort|TestAcceptIP|TestGetAllLocalIPs" ./web/backend/`
Expected: 4 PASS

- [ ] **Step 5: 提交**

```bash
git add web/backend/tls.go web/backend/tls_test.go
git commit -m "$(cat <<'EOF'
feat: TLS 核心纯函数 — shouldStartTLS / validateTLSPort / acceptIP / getAllLocalIPs

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code
EOF
)"
```

---

### Task 2: 证书生成 — generateSelfSignedCert + tlsMeta

**Files:**
- Modify: `web/backend/tls.go`（追加）
- Modify: `web/backend/tls_test.go`（追加）

- [ ] **Step 1: 写证书生成单测**

在 `tls_test.go` 追加：

```go
import (
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"
)

func TestGenerateSelfSignedCert(t *testing.T) {
	ips := []net.IP{net.ParseIP("192.168.1.1")}
	certPEM, keyPEM, fallback, err := generateSelfSignedCert(ips)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if fallback {
		t.Error("should not be fallback with normal clock")
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		t.Fatal("empty output")
	}

	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatal("not a valid PEM certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	if !cert.NotAfter.After(cert.NotBefore) {
		t.Error("NotAfter must be after NotBefore")
	}

	// Verify 127.0.0.1 in SAN
	foundV4 := false
	for _, ip := range cert.IPAddresses {
		if ip.Equal(net.IPv4(127, 0, 0, 1)) {
			foundV4 = true
		}
	}
	if !foundV4 {
		t.Error("127.0.0.1 must be in SAN")
	}

	// Verify key PEM
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil || keyBlock.Type != "EC PRIVATE KEY" {
		t.Fatal("not a valid EC private key")
	}
}

func TestGenerateSelfSignedCertMultipleIPs(t *testing.T) {
	ips := []net.IP{
		net.ParseIP("192.168.1.1"),
		net.ParseIP("10.0.0.1"),
	}
	certPEM, _, _, err := generateSelfSignedCert(ips)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	block, _ := pem.Decode(certPEM)
	cert, _ := x509.ParseCertificate(block.Bytes)
	// 2 loopback + 2 LAN = 4
	if len(cert.IPAddresses) < 4 {
		t.Errorf("expected >= 4 IPs in SAN, got %d", len(cert.IPAddresses))
	}
}

func TestGenerateSelfSignedCertEmptyIPs(t *testing.T) {
	certPEM, _, _, err := generateSelfSignedCert(nil)
	if err != nil {
		t.Fatalf("generate with nil IPs: %v", err)
	}
	block, _ := pem.Decode(certPEM)
	cert, _ := x509.ParseCertificate(block.Bytes)
	if len(cert.IPAddresses) < 2 {
		t.Errorf("expected >= 2 IPs (loopback), got %d", len(cert.IPAddresses))
	}
}

func TestClockFallback(t *testing.T) {
	_, _, fallback, err := generateSelfSignedCert(nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if fallback {
		t.Error("normal clock should not produce fallback")
	}
}
```

- [ ] **Step 2: 跑单测确认 FAIL**

Run: `go test -v -tags goolm,stdjson -run "TestGenerateSelfSignedCert" ./web/backend/`
Expected: FAIL

- [ ] **Step 3: 实现 `generateSelfSignedCert`**

在 `tls.go` 追加：

```go
import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	"github.com/sipeed/picoclaw/pkg/logger"
)

type tlsMeta struct {
	SchemaVersion int      `json:"schema_version"`
	SANs          []string `json:"sans"`
	ClockFallback bool     `json:"clock_fallback"`
	GeneratedAt   string   `json:"generated_at"`
}

const tlsSchemaVersion = 1

func generateSelfSignedCert(ips []net.IP) (certPEM, keyPEM []byte, clockFallback bool, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, false, fmt.Errorf("generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, false, fmt.Errorf("generate serial: %w", err)
	}

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

	sanIPs := append([]net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}, ips...)

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

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, false, fmt.Errorf("create certificate: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, false, fmt.Errorf("marshal key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	return certPEM, keyPEM, clockFallback, nil
}
```

- [ ] **Step 4: 跑单测确认 PASS**

Run: `go test -v -tags goolm,stdjson -run "TestGenerateSelfSignedCert" ./web/backend/`
Expected: 4 PASS

- [ ] **Step 5: 提交**

```bash
git add web/backend/tls.go web/backend/tls_test.go
git commit -m "$(cat <<'EOF'
feat: generateSelfSignedCert + tlsMeta 结构实现

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code
EOF
)"
```

---

### Task 3: 缓存管理 — load/save + matchesCurrentIPs + needsRegen

**Files:**
- Modify: `web/backend/tls.go`（追加）
- Modify: `web/backend/tls_test.go`（追加）

- [ ] **Step 1: 写缓存与匹配单测**

在 `tls_test.go` 追加：

```go
import (
	"os"
	"path/filepath"
	"testing"
)

func ipStrs(ss ...string) []string { return ss }

func TestMatchesCurrentIPsWith(t *testing.T) {
	meta := &tlsMeta{
		SchemaVersion: tlsSchemaVersion,
		SANs:          ipStrs("127.0.0.1", "::1", "192.168.1.1"),
	}
	// subset → true
	if !meta.matchesCurrentIPsWith(ipStrs("127.0.0.1", "::1", "192.168.1.1")) {
		t.Error("equal sets should match")
	}
	// fewer IPs → true
	if !meta.matchesCurrentIPsWith(ipStrs("127.0.0.1", "::1")) {
		t.Error("subset should match")
	}
	// new IP → false
	if meta.matchesCurrentIPsWith(ipStrs("127.0.0.1", "::1", "192.168.1.1", "10.0.0.1")) {
		t.Error("superset should not match")
	}
}

func TestNeedsRegen(t *testing.T) {
	// schema version mismatch
	metaOld := &tlsMeta{SchemaVersion: 0, SANs: ipStrs("127.0.0.1")}
	if !metaOld.needsRegen() {
		t.Error("schema version mismatch should trigger regen")
	}
	// clock fallback + normal time → regen
	metaFallback := &tlsMeta{
		SchemaVersion: tlsSchemaVersion,
		SANs:          ipStrs("127.0.0.1", "::1"),
		ClockFallback: true,
	}
	if !metaFallback.needsRegen() {
		t.Error("clock_fallback should trigger regen when clock is normal")
	}
	// everything OK → no regen
	ips := ipStrings(getAllLocalIPs())
	metaOK := &tlsMeta{
		SchemaVersion: tlsSchemaVersion,
		SANs:          ips,
		ClockFallback: false,
	}
	if metaOK.needsRegen() {
		t.Error("matching meta should not trigger regen")
	}
}

func TestSaveAndLoadTLSCache(t *testing.T) {
	dir := t.TempDir()
	certPEM := []byte("test-cert")
	keyPEM := []byte("test-key")
	meta := &tlsMeta{
		SchemaVersion: tlsSchemaVersion,
		SANs:          ipStrs("127.0.0.1", "::1"),
		GeneratedAt:   "2026-06-02T10:00:00Z",
	}

	if err := saveTLSCache(dir, certPEM, keyPEM, meta); err != nil {
		t.Fatalf("save: %v", err)
	}

	for _, name := range []string{"server.crt", "server.key", "meta.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("file %s not found: %v", name, err)
		}
	}

	loadedMeta, loadedCert, loadedKey, err := loadTLSCache(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if string(loadedCert) != string(certPEM) {
		t.Error("cert mismatch")
	}
	if string(loadedKey) != string(keyPEM) {
		t.Error("key mismatch")
	}
	if loadedMeta.SchemaVersion != meta.SchemaVersion {
		t.Error("meta mismatch")
	}
}

func TestLoadTLSCacheEmptyDir(t *testing.T) {
	dir := t.TempDir()
	_, _, _, err := loadTLSCache(dir)
	if err == nil {
		t.Error("should error on empty dir")
	}
}

func TestSaveTLSCacheCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "newsub")
	certPEM := []byte("test")
	keyPEM := []byte("test")
	meta := &tlsMeta{SchemaVersion: tlsSchemaVersion}

	if err := saveTLSCache(dir, certPEM, keyPEM, meta); err != nil {
		t.Fatalf("save should create dir: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Error("directory should be created")
	}
}

func TestFileWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	certPEM := []byte("atomic-test-cert")
	keyPEM := []byte("atomic-test-key")
	meta := &tlsMeta{SchemaVersion: tlsSchemaVersion}

	if err := saveTLSCache(dir, certPEM, keyPEM, meta); err != nil {
		t.Fatalf("save: %v", err)
	}

	// No .tmp files should remain
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("tmp file should not remain: %s", e.Name())
		}
	}

	// Data should be intact
	_, loadedCert, loadedKey, err := loadTLSCache(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if string(loadedCert) != string(certPEM) {
		t.Error("cert mismatch after atomic write")
	}
	if string(loadedKey) != string(keyPEM) {
		t.Error("key mismatch after atomic write")
	}
}
```

- [ ] **Step 2: 跑单测确认 FAIL**

Run: `go test -v -tags goolm,stdjson -run "TestMatches|TestNeedsRegen|TestSaveAndLoad|TestLoadTLSCacheEmptyDir|TestSaveTLSCacheCreatesDir|TestFileWriteAtomic" ./web/backend/`
Expected: FAIL

- [ ] **Step 3: 实现缓存函数**

在 `tls.go` 追加：

```go
import (
	"encoding/json"
	"os"
	"path/filepath"
)

const tlsCacheDir = "tls"

func tlsDir(home string) string {
	return filepath.Join(home, tlsCacheDir)
}

func (m *tlsMeta) matchesCurrentIPs() bool {
	return m.matchesCurrentIPsWith(ipStrings(getAllLocalIPs()))
}

func (m *tlsMeta) matchesCurrentIPsWith(current []string) bool {
	cachedSet := make(map[string]struct{}, len(m.SANs))
	for _, san := range m.SANs {
		cachedSet[san] = struct{}{}
	}
	for _, ip := range current {
		if _, ok := cachedSet[ip]; !ok {
			return false
		}
	}
	return true
}

func (m *tlsMeta) needsRegen() bool {
	if m.SchemaVersion != tlsSchemaVersion {
		return true
	}
	if m.ClockFallback && time.Now().After(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) {
		return true
	}
	if !m.matchesCurrentIPs() {
		return true
	}
	return false
}

func loadTLSCache(dir string) (*tlsMeta, []byte, []byte, error) {
	metaPath := filepath.Join(dir, "meta.json")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read meta: %w", err)
	}
	var meta tlsMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return nil, nil, nil, fmt.Errorf("parse meta: %w", err)
	}

	certPath := filepath.Join(dir, "server.crt")
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read cert: %w", err)
	}

	keyPath := filepath.Join(dir, "server.key")
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read key: %w", err)
	}

	return &meta, certPEM, keyPEM, nil
}

func saveTLSCache(dir string, certPEM, keyPEM []byte, meta *tlsMeta) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	// flock for concurrent protection
	lockPath := filepath.Join(dir, ".lock")
	unlock, err := lockFile(lockPath)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	defer unlock()

	// Write .tmp then rename (atomic on same filesystem)
	writeAndRename := func(name string, data []byte) error {
		tmpPath := filepath.Join(dir, name+".tmp")
		if err := os.WriteFile(tmpPath, data, 0600); err != nil {
			return err
		}
		return os.Rename(tmpPath, filepath.Join(dir, name))
	}

	if err := writeAndRename("server.crt", certPEM); err != nil {
		return fmt.Errorf("write cert: %w", err)
	}
	if err := writeAndRename("server.key", keyPEM); err != nil {
		return fmt.Errorf("write key: %w", err)
	}

	metaData, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}
	if err := writeAndRename("meta.json", metaData); err != nil {
		return fmt.Errorf("write meta: %w", err)
	}

	return nil
}

func ipStrings(ips []net.IP) []string {
	s := make([]string, len(ips))
	for i, ip := range ips {
		s[i] = ip.String()
	}
	return s
}
```

- [ ] **Step 4: 跑单测确认 PASS**

Run: `go test -v -tags goolm,stdjson -run "TestMatches|TestNeedsRegen|TestSaveAndLoad|TestLoadTLSCacheEmptyDir|TestSaveTLSCacheCreatesDir|TestFileWriteAtomic" ./web/backend/`
Expected: 6 PASS

- [ ] **Step 5: 提交**

```bash
git add web/backend/tls.go web/backend/tls_test.go
git commit -m "$(cat <<'EOF'
feat: TLS 缓存管理 — load/save/matchesCurrentIPsWith/needsRegen + flock 集成

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code
EOF
)"
```

---

### Task 4: 文件锁 — Unix + Windows 跨平台

**Files:**
- Create: `web/backend/tls_lock_unix.go`
- Create: `web/backend/tls_lock_windows.go`

- [ ] **Step 1: Unix lock**

`web/backend/tls_lock_unix.go`:

```go
//go:build !windows

package main

import (
	"os"
	"syscall"
)

func lockFile(lockPath string) (func(), error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("flock: %w", err)
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
```

- [ ] **Step 2: Windows lock**

`web/backend/tls_lock_windows.go`:

```go
//go:build windows

package main

import "fmt"

func lockFile(lockPath string) (func(), error) {
	// Windows: .tmp + Rename in saveTLSCache is sufficient
	// Launcher on Windows is rarely used for LAN TLS scenarios.
	// Full implementation via LockFileEx can be added later.
	return func() {}, nil
}
```

- [ ] **Step 3: 编译验证双平台**

Run:
```bash
GOOS=linux go build -tags goolm,stdjson ./web/backend/ 2>&1
GOOS=windows go build -tags goolm,stdjson ./web/backend/ 2>&1
```
Expected: both succeed

- [ ] **Step 4: 提交**

```bash
git add web/backend/tls_lock_unix.go web/backend/tls_lock_windows.go
git commit -m "$(cat <<'EOF'
feat: TLS 文件锁 — Unix flock + Windows stub

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code
EOF
)"
```

---

### Task 5: ensureTLS 编排 + main.go 集成

**Files:**
- Modify: `web/backend/tls.go`（追加 ensureTLS）
- Modify: `web/backend/main.go`（flag 解析 + HTTPS server + 控制台打印）
- Modify: `web/backend/tls_test.go`（追加）

- [ ] **Step 1: 写 ensureTLS 和 flag 单测**

在 `tls_test.go` 追加：

```go
func TestHTTPSNotStartedWithoutPublic(t *testing.T) {
	if shouldStartTLS(false, false) {
		t.Error("TLS should not start without -public")
	}
}

func TestHTTPSNotStartedWhenDisabled(t *testing.T) {
	if shouldStartTLS(true, true) {
		t.Error("TLS should not start with -no-tls")
	}
}

func TestTLSPortEqualsHTTPPort(t *testing.T) {
	if err := validateTLSPort(18800, 18800); err == nil {
		t.Error("same port should error")
	}
}

func TestClockFallbackPersisted(t *testing.T) {
	meta := &tlsMeta{
		SchemaVersion: tlsSchemaVersion,
		SANs:          ipStrs("127.0.0.1", "::1"),
		ClockFallback: true,
	}
	if !meta.needsRegen() {
		t.Error("clock_fallback meta should trigger regen when clock is normal")
	}
}

func TestConcurrentSaveTLSCache(t *testing.T) {
	dir := t.TempDir()
	home := dir
	t.Setenv("PICOCLAW_HOME", home)

	// ensureTLS called N times concurrently should produce consistent cache
	done := make(chan error, 5)
	for i := 0; i < 5; i++ {
		go func() {
			// Simulate ensureTLS internal cache path — use saveTLSCache which has flock
			certPEM, keyPEM, fallback, _ := generateSelfSignedCert(nil)
			meta := &tlsMeta{
				SchemaVersion: tlsSchemaVersion,
				SANs:          ipStrs("127.0.0.1", "::1"),
			}
			_ = fallback
			done <- saveTLSCache(tlsDir(home), certPEM, keyPEM, meta)
		}()
	}
	for i := 0; i < 5; i++ {
		if err := <-done; err != nil {
			t.Errorf("concurrent save: %v", err)
		}
	}

	// Cache must be loadable after all goroutines
	_, _, _, err := loadTLSCache(tlsDir(home))
	if err != nil {
		t.Fatalf("concurrent writes should leave valid cache: %v", err)
	}
}
```

- [ ] **Step 2: 实现 `ensureTLS`**

在 `tls.go` 追加：

```go
import (
	"crypto/tls"
	"net/http"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/netbind"
)

func ensureTLS(hostInput string, effectivePublic bool, tlsPort string, home string) (netbind.OpenResult, *tls.Config, error) {
	dir := tlsDir(home)

	// Try load cache
	meta, certPEM, keyPEM, loadErr := loadTLSCache(dir)
	if loadErr != nil || meta.needsRegen() {
		if loadErr != nil {
			logger.InfoC("web", "TLS 缓存不存在或损坏，生成新证书")
		} else if meta.needsRegen() {
			logger.InfoC("web", "TLS 证书需更新（IP 变化/时钟同步/schema 升级），重新生成")
		}

		ips := getAllLocalIPs()
		var fallback bool
		var genErr error
		certPEM, keyPEM, fallback, genErr = generateSelfSignedCert(ips) // 注意: = 不是 :=，避免 shadow 外层变量
		if genErr != nil {
			return netbind.OpenResult{}, nil, fmt.Errorf("生成 TLS 证书失败: %w", genErr)
		}

		meta = &tlsMeta{
			SchemaVersion: tlsSchemaVersion,
			SANs:          ipStrings(ips),
			ClockFallback: fallback,
			GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		}

		if err := saveTLSCache(dir, certPEM, keyPEM, meta); err != nil {
			return netbind.OpenResult{}, nil, fmt.Errorf("保存 TLS 缓存失败: %w", err)
		}
		logger.InfoC("web", "TLS 证书已生成并写入缓存")
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return netbind.OpenResult{}, nil, fmt.Errorf("解析 TLS 密钥对失败: %w", err)
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	defaultMode := netbind.DefaultLoopback
	if effectivePublic && strings.TrimSpace(hostInput) == "" {
		defaultMode = netbind.DefaultAny
	}
	plan, planErr := netbind.BuildPlan(hostInput, defaultMode)
	if planErr != nil {
		return netbind.OpenResult{}, nil, fmt.Errorf("构建 TLS 绑定计划失败: %w", planErr)
	}
	result, openErr := netbind.OpenPlan(plan, tlsPort)
	if openErr != nil {
		return netbind.OpenResult{}, nil, fmt.Errorf("HTTPS 端口 %s 绑定失败: %w", tlsPort, openErr)
	}

	return result, tlsCfg, nil
}
```

- [ ] **Step 3: 改 `main.go` — flag 解析**

在 `func main()` flag 声明区域（`port := flag.String(...)` 附近）：

```go
noTLS := flag.Bool("no-tls", false, "Disable HTTPS (even when -public is set)")
tlsPort := flag.String("tls-port", "18443", "HTTPS port to listen on")
```

flag.Usage 追加：

```go
fmt.Fprintf(os.Stderr, "  %s -public -tls-port 18443 ./config.json\n", os.Args[0])
fmt.Fprintf(os.Stderr, "      Start with HTTPS on port 18443\n")
```

flag 解析后立即校验 tlsPort（无论是否启用 HTTPS，保证 flag 值合法）：

```go
tlsPortNum, err := strconv.Atoi(*tlsPort)
if err != nil || tlsPortNum < 1 || tlsPortNum > 65535 {
    if err == nil {
        err = errors.New("must be in range 1-65535")
    }
    logger.Fatalf("Invalid TLS port %q: %v", *tlsPort, err)
}
```

- [ ] **Step 4: 改 `main.go` — HTTPS 启动逻辑**

在 `openResult, err := openLauncherListeners(...)` 之后、`servers = make(...)` 之前，插入：

```go
var tlsListeners []net.Listener
var httpsAddr string
if shouldStartTLS(effectivePublic, *noTLS) {
	if err := validateTLSPort(portNum, tlsPortNum); err != nil {
		logger.Fatalf("TLS 配置错误: %v", err)
	}
	tlsResult, tlsCfg, tlsErr := ensureTLS(hostInput, effectivePublic, *tlsPort, picoHome)
	if tlsErr != nil {
		logger.ErrorC("web", fmt.Sprintf("HTTPS 启动失败，仅 HTTP 可用: %v", tlsErr))
	} else {
		tlsListeners = tlsResult.Listeners
		// Compute advertise address for HTTPS (same logic as HTTP)
		httpsHost := openResult.ProbeHost
		if hasWildcardBindHosts(tlsResult.BindHosts) {
			if ip := advertiseIPForWildcardBindHosts(tlsResult.BindHosts); ip != "" {
				httpsHost = ip
			}
		}
		httpsAddr = fmt.Sprintf("https://%s", net.JoinHostPort(httpsHost, *tlsPort))

		for _, ln := range tlsResult.Listeners {
			tlsSrv := &http.Server{
				Handler:   handler, // 复用 HTTP 同款 handler（grep -n "Recoverer" main.go 可定位）
				TLSConfig: tlsCfg,
			}
			servers = append(servers, tlsSrv)
			go func(s *http.Server, l net.Listener) {
				logger.InfoC("web", fmt.Sprintf("HTTPS 监听: https://%s", l.Addr().String()))
				if serveErr := s.ServeTLS(l, "", ""); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
					logger.Fatalf("HTTPS server failed on %s: %v", l.Addr().String(), serveErr)
				}
			}(tlsSrv, ln)
		}
	}
}
```

注意：`handler` 变量定义于 `handler := middleware.Recoverer(...)`（`grep -n "Recoverer" main.go` 可定位），HTTPS 和 HTTP 共用同一 handler。

- [ ] **Step 5: 改 `main.go` — 控制台打印 HTTPS 地址**

HTTPS 打印块必须紧贴在 HTTP 打印块的 `}` 之前，与 HTTP 共享同一个 `consoleHosts` 局部变量（该变量在 `if !*noBrowser` 分支内）：

```go
// 在已有的 HTTP 打印循环（for _, host := range consoleHosts { ... }）之后、
// 该 if 块的结束 } 之前，插入：
if len(tlsListeners) > 0 {
    fmt.Println()
    fmt.Println("  HTTPS (语音功能):")
    fmt.Println()
    for _, h := range consoleHosts {
        fmt.Printf("    >> https://%s <<\n", net.JoinHostPort(h, *tlsPort))
    }
    fmt.Println()
}
```

注意：不能写到 `if !*noBrowser` 分支外面——否则 `consoleHosts` 未定义。

- [ ] **Step 6: 编译验证**

Run: `go build -tags goolm,stdjson ./web/backend/`
Expected: compile PASS

- [ ] **Step 7: 跑全量单测**

Run: `go test -v -tags goolm,stdjson ./web/backend/...`
Expected: all PASS

- [ ] **Step 8: 提交**

```bash
git add web/backend/tls.go web/backend/main.go web/backend/tls_test.go
git commit -m "$(cat <<'EOF'
feat: ensureTLS 编排 + main.go 集成 HTTPS server + 控制台打印

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code
EOF
)"
```

---

### Task 6: `/api/system/version` 新增 `http_url` / `https_url`

**Files:**
- Modify: `web/backend/api/version.go`
- Modify: `web/backend/api/router.go`
- Modify: `web/backend/main.go`

- [ ] **Step 1: 扩展 `systemVersionResponse`**

`web/backend/api/version.go`:

```go
type systemVersionResponse struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit,omitempty"`
	BuildTime string `json:"build_time,omitempty"`
	GoVersion string `json:"go_version"`
	HTTPURL   string `json:"http_url,omitempty"`
	HTTPSURL  string `json:"https_url,omitempty"`
}
```

- [ ] **Step 2: `Handler` 新增字段 + setter**

实施前确认：`grep -n "type Handler struct" web/backend/api/` 找到 Handler 定义文件（当前假设为 router.go）。

在 Handler struct 尾部追加：

```go
httpURL  string
httpsURL string
```

新增 setter method：

```go
func (h *Handler) SetLauncherURLs(httpURL, httpsURL string) {
	h.httpURL = httpURL
	h.httpsURL = httpsURL
}
```

- [ ] **Step 3: `handleGetVersion` 填充 URL**

`web/backend/api/version.go` 的 `handleGetVersion`:

```go
func (h *Handler) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	versionInfo := h.resolveSystemVersionInfo(r.Context())
	versionInfo.HTTPURL = h.httpURL
	versionInfo.HTTPSURL = h.httpsURL

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(versionInfo); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
```

- [ ] **Step 4: `main.go` 调用 setter**

**关键时序：** `SetLauncherURLs` 必须放在 `serverAddr` 和 `httpsAddr` 都计算完成之后。`serverAddr` 在 `main.go` 原有代码中定义（`serverAddr = fmt.Sprintf(...)`），`httpsAddr` 在 Task 5 Step 4 中设置。

**插入位置：** HTTP server goroutine 启动之前（原 `servers = make([]*http.Server, 0, len(listeners))` 之前）。不在 `RegisterRoutes` 之前——路由注册时的 url 值不重要，Handler 字段在请求到来时被动态读取。

```go
apiHandler.SetLauncherURLs(serverAddr, httpsAddr)
// 紧接着是 servers = make(...) → HTTP server 启动循环
```

`httpsAddr` 在 TLS 未启动时为空字符串。

- [ ] **Step 5: 编译验证 + 单测**

Run:
```bash
go build -tags goolm,stdjson ./web/backend/
go test -v -tags goolm,stdjson -run "TestGetVersion|TestSystemVersion" ./web/backend/api/
```
Expected: compile + PASS

- [ ] **Step 6: 提交**

```bash
git add web/backend/api/version.go web/backend/api/router.go web/backend/main.go
git commit -m "$(cat <<'EOF'
feat: /api/system/version 新增 http_url / https_url 字段

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code
EOF
)"
```

---

### Task 7: 前端 — chat-composer isSecureContext 检测 + api/voice 拉取 URL

**Files:**
- Modify: `web/frontend/src/components/chat/chat-composer.tsx`
- Modify: `web/frontend/src/api/voice.ts`
实现前确认：`chat-composer.tsx` 麦克风按钮用 emoji，不引入 lucide-react

- [ ] **Step 1: `api/voice.ts` 新增 `fetchVersionURLs`**

```typescript
// 向已有 import 块追加 launcherFetch（检查是否已 import）
import { launcherFetch } from "./http"

export async function fetchVersionURLs(): Promise<{ http_url: string; https_url: string }> {
  const res = await launcherFetch("/api/system/version")
  if (!res.ok) return { http_url: "", https_url: "" }
  const data = await res.json()
  return {
    http_url: data.http_url || "",
    https_url: data.https_url || "",
  }
}
```

- [ ] **Step 2: `chat-composer.tsx` — 🎤 按钮 isSecure 检测**

向已有 React import 块按需追加 `useState` / `useEffect`（若尚未导入）。

`ChatComposer` 函数体顶部新增：

```typescript
const [httpsUrl, setHttpsUrl] = useState("")
const [httpsUrlLoaded, setHttpsUrlLoaded] = useState(false)

useEffect(() => {
  if (typeof window !== "undefined" && !window.isSecureContext) {
    fetchVersionURLs().then((urls) => {
      setHttpsUrl(urls.https_url || `https://${location.hostname}:18443`)
      setHttpsUrlLoaded(true)
    }).catch(() => {
      // fetch 失败时按默认端口 18443 显示提示，自定义 --tls-port 下可能不准
      setHttpsUrl(`https://${location.hostname}:18443`)
      setHttpsUrlLoaded(true)
    })
  }
}, []) // eslint-disable-line react-hooks/exhaustive-deps — state setters are stable refs
```

🎤 按钮区域（保留原有 emoji 风格，仅追加 isSecure 分支）：

```tsx
const isSecure = typeof window !== "undefined" && window.isSecureContext

{streamingAvailable && isSecure ? (
  // 原有正常按钮 — 不变，点🎤 触发 setShowRecorder(true)
  <button ... onClick={() => setShowRecorder((v) => !v)}>
    <span>{showRecorder ? "🎙️" : "🎤"}</span>
  </button>
) : streamingAvailable && !isSecure ? (
  // 非安全上下文：disabled + tooltip，不挂载 VoiceRecorder
  <button
    disabled
    title={httpsUrlLoaded ? `语音录音需通过 HTTPS 访问。请访问 ${httpsUrl}` : "语音录音需通过 HTTPS 访问"}
  >
    <span>🔇</span>
  </button>
) : null}
```

关键：`!isSecure` 时 `setShowRecorder(true)` 不会被调用 → VoiceRecorder 不会挂载 → UI 不乱。

- [ ] **Step 3: 类型检查**

Run: `cd web/frontend && npx tsc --noEmit`
Expected: zero errors

- [ ] **Step 4: 提交**

```bash
git add web/frontend/src/
git commit -m "$(cat <<'EOF'
feat: 前端 isSecureContext 检测 — chat-composer disabled 麦克风 + tooltip

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code
EOF
)"
```

---

### Task 8: 部署脚本 + 全量构建验收

**Files:**
- Modify: `scripts/deploy-rk3506.sh`

- [ ] **Step 1: 部署脚本打印 HTTPS**

```bash
# 替换最终 echo 块:
echo "部署完成！"
echo "  HTTP:  http://${DEVICE_IP}:18800"
echo "  HTTPS: https://${DEVICE_IP}:18443  (语音功能)"
```

- [ ] **Step 2: 全量构建**

Run: `make build && make build-launcher`
Expected: both succeed

- [ ] **Step 3: 全量单测**

Run:
```bash
go test -v -tags goolm,stdjson ./web/backend/...
cd web/frontend && npx tsc --noEmit
```
Expected: all PASS

- [ ] **Step 4: 提交**

```bash
git add scripts/deploy-rk3506.sh
git commit -m "$(cat <<'EOF'
feat: 部署脚本打印 HTTPS 地址

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code
EOF
)"
```

---

### Task 9: 敏捷文档收尾

- [ ] **Step 1: 更新 BACKLOG.md**

BL-010 从「待规划」移到「已交付」，标注 `已完成于 sprint_007.md`。

- [ ] **Step 2: 更新 sprint_007.md**

Sprint 007 已存在（BACKLOG.md 顶部声明），更新内容：
- 补「创建」日期
- 添加 BL-010 任务清单（引用设计文档和实现计划路径）
- 补完成时间
- 状态改为「已完成」

- [ ] **Step 3: 提交**

```bash
git add .scrum/
git commit -m "$(cat <<'EOF'
docs: Sprint 007 完成 — BL-010 Launcher TLS + 自签名证书自动签发

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code
EOF
)"
```

---

## 执行顺序

```
Task 1 → Task 2 → Task 3 → Task 4 → Task 5 → Task 6 → Task 7 → Task 8 → Task 9
```

Task 1-4 独立（可连续执行），Task 5 依赖 1-4，Task 6-7 依赖 5，Task 8 全量验证，Task 9 收尾。
