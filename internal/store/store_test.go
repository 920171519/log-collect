package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"logcollector/internal/model"
)

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", ConfigFileName)
	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("initial config was not created: %v", err)
	}
	cfg := st.Get()
	cfg.DownloadDir = filepath.Join(t.TempDir(), "downloads")
	combo := model.NewCombination("组合 A")
	combo.ID = "combo-store-test"
	combo.Cloud = model.HostConfig{IP: "10.1.2.3", Port: 22, Username: "root", Password: "cloud-secret"}
	combo.DeviceA.IP = "192.168.1.10"
	combo.DeviceA.Password = "a-secret"
	combo.DeviceB.IP = ""
	cfg.Combinations = []model.Combination{combo}
	saved, err := st.Save(cfg)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.Combinations[0].Cloud.Password != "cloud-secret" {
		t.Fatal("saved config lost password")
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("config permissions are too broad: %o", info.Mode().Perm())
	}

	reloaded, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got := reloaded.Get()
	if len(got.Combinations) != 1 {
		t.Fatalf("combination count = %d", len(got.Combinations))
	}
	if got.Combinations[0].Cloud.Password != "cloud-secret" ||
		got.Combinations[0].DeviceA.Password != "a-secret" {
		t.Fatal("password was not preserved across reload")
	}
	if reloaded.LoadError() != "" {
		t.Fatalf("unexpected load error: %s", reloaded.LoadError())
	}
}

func TestStoreGeneratesMissingCombinationIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), ConfigFileName)
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := st.Get()
	combo := model.NewCombination("no id")
	combo.ID = ""
	cfg.Combinations = []model.Combination{combo}
	saved, err := st.Save(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(saved.Combinations[0].ID, "combo-") {
		t.Fatalf("missing pair ID was not generated: %q", saved.Combinations[0].ID)
	}
}

func TestStoreRecoversFromMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), ConfigFileName)
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.LoadError() == "" {
		t.Fatal("expected a load error for malformed JSON")
	}
	recovered, err := st.Save(st.Get())
	if err != nil {
		t.Fatalf("save recovered config: %v", err)
	}
	if st.LoadError() != "" {
		t.Fatalf("load error was not cleared after save: %s", st.LoadError())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded model.AppConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("recovered file is not valid JSON: %v", err)
	}
	if len(recovered.Combinations) != 0 {
		t.Fatalf("expected empty combinations after recovery, got %d", len(recovered.Combinations))
	}
}

func TestStoreAllowsEmptyConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), ConfigFileName)
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := st.Get()
	cfg.Combinations = []model.Combination{}
	if _, err := st.Save(cfg); err != nil {
		t.Fatalf("empty configuration should save: %v", err)
	}
}

func TestStoreSaveValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), ConfigFileName)
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := st.Get()
	cfg.CloudStageDir = "relative/path"
	if _, err := st.Save(cfg); err == nil {
		t.Fatal("expected validation error for relative remote path")
	}
}
