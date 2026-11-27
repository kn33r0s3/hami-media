//go:build linux

package main

import "testing"

func TestLinuxCPUModelArm64Fallback(t *testing.T) {
	cpuinfo := "processor\t: 0\n" +
		"BogoMIPS\t: 48.00\n" +
		"Features\t: fp asimd aes\n" +
		"CPU implementer\t: 0x61\n" +
		"CPU architecture\t: 8\n"
	got := linuxCPUModel(cpuinfo)
	if got == "" {
		t.Fatal("linuxCPUModel() returned empty for arm64 cpuinfo")
	}
	if got != "ARM64 architecture 8" {
		t.Fatalf("linuxCPUModel() = %q, want %q", got, "ARM64 architecture 8")
	}
}
