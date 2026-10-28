package collector

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"logcollector/internal/model"
)

func TestShellQuoteAndCollectionScript(t *testing.T) {
	marker := "DONE_MARKER_WITH_'QUOTE"
	commands := "printf 'hello world\\n'\n" +
		"printf \"quoted: %s\\n\" \"a'b\"\n" +
		"printf 'error-line\\n' >&2\n"
	command := BuildCollectionCommand(commands, marker)
	output, err := exec.Command("/bin/sh", "-c", command).CombinedOutput()
	if err != nil {
		t.Fatalf("execute collection command: %v\n%s", err, output)
	}
	text := string(output)
	for _, want := range []string{"hello world", "quoted: a'b", "error-line"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
	if !strings.Contains(text, marker+":0") {
		t.Fatalf("output missing status marker %q:\n%s", marker+":0", text)
	}
}

func TestBuildCollectionScriptPreservesFailureStatus(t *testing.T) {
	command := BuildCollectionCommand("echo before-failure\nfalse\n", "FAIL_MARKER")
	output, err := exec.Command("/bin/sh", "-c", command).CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit status, output=%s", output)
	}
	if !strings.Contains(string(output), "FAIL_MARKER:1") {
		t.Fatalf("expected marker with exit status 1, got:\n%s", output)
	}
}

func TestBuildArchiveCommandHandlesSpacesAndQuotes(t *testing.T) {
	dir := t.TempDir()
	logsDir := filepath.Join(dir, "log dir 'quoted'")
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logsDir, "a log.txt"), []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	tempDir := filepath.Join(dir, "device temp")
	archivePath := filepath.Join(tempDir, "archive.tar.gz")
	command := BuildArchiveCommand(tempDir, archivePath, []string{logsDir})
	output, err := exec.Command("/bin/sh", "-c", command).CombinedOutput()
	if err != nil {
		t.Fatalf("archive command failed: %v\n%s\ncommand: %s", err, output, command)
	}
	if info, err := os.Stat(archivePath); err != nil || info.Size() == 0 {
		t.Fatalf("archive not created: %v", err)
	}
	contents := collectArchiveText(t, archivePath)
	if !strings.Contains(contents, "payload") {
		t.Fatalf("archive does not contain log payload:\n%s", contents)
	}
}

func TestExpandArchiveNameIsSafe(t *testing.T) {
	combo := model.Combination{
		ID:   "combo-12345678",
		Name: "测试/组合: 01",
	}
	name := ExpandArchiveName("../../{combo}-{device}-{timestamp}", combo, "A")
	if strings.ContainsAny(name, "/\\") {
		t.Fatalf("archive name contains path separator: %q", name)
	}
	if strings.Contains(name, "..") {
		t.Fatalf("archive name retains traversal: %q", name)
	}
	if !strings.HasSuffix(strings.ToLower(name), ".tar.gz") {
		t.Fatalf("archive name should end with .tar.gz: %q", name)
	}
	if !strings.Contains(name, "A") {
		t.Fatalf("archive name should contain device: %q", name)
	}
	dir := SafeComboDir("CON", "combo-abcdef123456")
	if !strings.HasPrefix(dir, "_CON-") {
		t.Fatalf("Windows reserved combo name not sanitized: %q", dir)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := map[int64]string{
		0:       "0 B",
		512:     "512 B",
		1536:    "1.5 KB",
		1024:    "1.0 KB",
		1048576: "1.0 MB",
	}
	for input, want := range cases {
		if got := FormatBytes(input); got != want {
			t.Fatalf("FormatBytes(%d) = %q, want %q", input, got, want)
		}
	}
}
