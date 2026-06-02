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
