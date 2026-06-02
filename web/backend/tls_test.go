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
