package main

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
)

type Blocker struct {
	dryRun    bool
	allowlist []*net.IPNet
}

func NewBlocker(cfg Config) (*Blocker, error) {
	blocker := &Blocker{dryRun: cfg.BlockerDryRun}
	for _, cidr := range cfg.Allowlist {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			ip := net.ParseIP(cidr)
			if ip == nil {
				return nil, fmt.Errorf("invalid allowlist entry %q", cidr)
			}
			if ip.To4() != nil {
				cidr = cidr + "/32"
			} else {
				cidr = cidr + "/128"
			}
			_, network, err = net.ParseCIDR(cidr)
			if err != nil {
				return nil, fmt.Errorf("invalid allowlist entry %q: %w", cidr, err)
			}
		}
		blocker.allowlist = append(blocker.allowlist, network)
	}
	return blocker, nil
}

func (b *Blocker) SelfCheck() error {
	if b.dryRun {
		return nil
	}
	if _, err := exec.LookPath("iptables"); err != nil {
		return fmt.Errorf("iptables not found: %w", err)
	}
	return runCommand("iptables", "-L", "-n")
}

func (b *Blocker) IsAllowedIP(value string) bool {
	ip := net.ParseIP(value)
	if ip == nil {
		return true
	}
	for _, network := range b.allowlist {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func (b *Blocker) Block(ip string) error {
	if b.IsAllowedIP(ip) {
		return fmt.Errorf("refusing to block allowlisted or invalid IP %q", ip)
	}
	if b.dryRun {
		return nil
	}
	if err := runCommand("iptables", "-C", "INPUT", "-s", ip, "-j", "DROP"); err == nil {
		return nil
	}
	return runCommand("iptables", "-I", "INPUT", "-s", ip, "-j", "DROP")
}

func (b *Blocker) Unblock(ip string) error {
	if b.dryRun {
		return nil
	}
	if err := runCommand("iptables", "-C", "INPUT", "-s", ip, "-j", "DROP"); err != nil {
		return nil
	}
	return runCommand("iptables", "-D", "INPUT", "-s", ip, "-j", "DROP")
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%s %v failed: %w: %s", name, args, err, stderr.String())
		}
		return fmt.Errorf("%s %v failed: %w", name, args, err)
	}
	return nil
}
