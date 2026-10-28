package model

import (
	"strings"
	"testing"
)

func validCombination() Combination {
	combo := NewCombination("测试组合")
	combo.ID = "combo-1"
	combo.Cloud = HostConfig{IP: "10.0.0.1", Port: 22, Username: "root", Password: "cloud"}
	combo.DeviceA.IP = "192.168.1.10"
	combo.DeviceA.Password = "a"
	combo.DeviceB.IP = "192.168.1.20"
	combo.DeviceB.Password = "b"
	return combo
}

func TestCombinationValidationWithOptionalDeviceB(t *testing.T) {
	combo := validCombination()
	if err := combo.Validate(); err != nil {
		t.Fatalf("complete combination should validate: %v", err)
	}
	combo.DeviceB = NewCombination("").DeviceB
	combo.DeviceB.IP = ""
	if err := combo.Validate(); err != nil {
		t.Fatalf("combination without device B should validate: %v", err)
	}
}

func TestCombinationValidationRejectsIncompleteDeviceB(t *testing.T) {
	combo := validCombination()
	combo.DeviceB.Password = ""
	err := combo.Validate()
	if err == nil || !strings.Contains(err.Error(), "设备 B") {
		t.Fatalf("expected device B validation error, got %v", err)
	}
}

func TestEndMarkerMustBeSingleLine(t *testing.T) {
	combo := validCombination()
	combo.DeviceA.EndMarker = "DONE\nINJECTED"
	err := combo.Validate()
	if err == nil || !strings.Contains(err.Error(), "一行文本") {
		t.Fatalf("expected marker validation error, got %v", err)
	}
}

func TestIPWithWhitespaceIsRejected(t *testing.T) {
	combo := validCombination()
	combo.Cloud.IP = "10.0.0.1 10.0.0.2"
	if err := combo.Validate(); err == nil {
		t.Fatal("expected whitespace IP validation error")
	}
}

func TestEmptyConfigCanBeSaved(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("empty configuration should be saveable: %v", err)
	}
}

func TestNormalizePreservesDisabledDeviceBFields(t *testing.T) {
	cfg := DefaultConfig()
	combo := validCombination()
	combo.DeviceB.IP = " "
	combo.DeviceB.Password = "keep-this-secret"
	combo.DeviceB.Commands = "echo keep-this-recipe"
	cfg.Combinations = []Combination{combo}
	cfg.Normalize()
	if cfg.Combinations[0].DeviceB.IP != "" {
		t.Fatalf("empty B IP should stay empty, got %q", cfg.Combinations[0].DeviceB.IP)
	}
	if cfg.Combinations[0].DeviceB.EndMarker == "" {
		t.Fatal("disabled B should receive default marker")
	}
	if cfg.Combinations[0].DeviceB.Password != "keep-this-secret" {
		t.Fatal("disabled B password should be preserved")
	}
	if cfg.Combinations[0].DeviceB.Commands != "echo keep-this-recipe" {
		t.Fatal("disabled B commands should be preserved")
	}
	if cfg.Combinations[0].Name != "测试组合" {
		t.Fatalf("name changed unexpectedly: %q", cfg.Combinations[0].Name)
	}
}

func TestArchiveTemplateRequiresDevicePlaceholder(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ArchiveNameTemplate = "logs.tar.gz"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for missing {device}")
	}
}

func TestDefaultConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Version != ConfigVersion {
		t.Fatalf("version = %d", cfg.Version)
	}
	if cfg.TimeoutMinutes != DefaultTimeoutMinutes {
		t.Fatalf("timeout = %d", cfg.TimeoutMinutes)
	}
	if !strings.Contains(cfg.ArchiveNameTemplate, "{device}") {
		t.Fatalf("default archive template missing device placeholder")
	}
}

func TestJoinHostPort(t *testing.T) {
	if got := JoinHostPort("127.0.0.1", 2222); got != "127.0.0.1:2222" {
		t.Fatalf("JoinHostPort = %q", got)
	}
	if got := JoinHostPort("::1", 22); got != "[::1]:22" {
		t.Fatalf("IPv6 JoinHostPort = %q", got)
	}
}
