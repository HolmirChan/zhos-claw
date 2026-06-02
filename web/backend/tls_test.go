package main

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
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
	if !acceptIP(net.ParseIP("192.168.1.1")) {
		t.Error("192.168.1.1 should be accepted")
	}
	if acceptIP(net.ParseIP("169.254.1.1")) {
		t.Error("169.254.1.1 link-local should be rejected")
	}
	if acceptIP(net.ParseIP("fe80::1")) {
		t.Error("fe80::1 link-local should be rejected")
	}
	if acceptIP(net.ParseIP("2001:db8::1")) {
		t.Error("2001:db8::1 GUA should be rejected")
	}
	if acceptIP(net.ParseIP("fd00::1")) {
		t.Error("fd00::1 ULA should be rejected")
	}
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

	foundV4 := false
	for _, ip := range cert.IPAddresses {
		if ip.Equal(net.IPv4(127, 0, 0, 1)) {
			foundV4 = true
		}
	}
	if !foundV4 {
		t.Error("127.0.0.1 must be in SAN")
	}

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

func ipStrs(ss ...string) []string { return ss }

func TestMatchesCurrentIPsWith(t *testing.T) {
	meta := &tlsMeta{
		SchemaVersion: tlsSchemaVersion,
		SANs:          ipStrs("127.0.0.1", "::1", "192.168.1.1"),
	}
	if !meta.matchesCurrentIPsWith(ipStrs("127.0.0.1", "::1", "192.168.1.1")) {
		t.Error("equal sets should match")
	}
	if !meta.matchesCurrentIPsWith(ipStrs("127.0.0.1", "::1")) {
		t.Error("subset should match")
	}
	if meta.matchesCurrentIPsWith(ipStrs("127.0.0.1", "::1", "192.168.1.1", "10.0.0.1")) {
		t.Error("superset should not match")
	}
}

func TestNeedsRegen(t *testing.T) {
	metaOld := &tlsMeta{SchemaVersion: 0, SANs: ipStrs("127.0.0.1")}
	if !metaOld.needsRegen() {
		t.Error("schema version mismatch should trigger regen")
	}
	metaFallback := &tlsMeta{
		SchemaVersion: tlsSchemaVersion,
		SANs:          ipStrs("127.0.0.1", "::1"),
		ClockFallback: true,
	}
	if !metaFallback.needsRegen() {
		t.Error("clock_fallback should trigger regen when clock is normal")
	}
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

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("tmp file should not remain: %s", e.Name())
		}
	}

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
