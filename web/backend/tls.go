package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
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
		return true
	}
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
			}
		}
	}
	return ips
}

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

	unlock, err := lockFile(filepath.Join(dir, ".lock"))
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	defer unlock()

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
