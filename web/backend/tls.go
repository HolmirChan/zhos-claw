package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
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
