package collector

import (
	"fmt"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"logcollector/internal/model"
)

// ShellQuote returns a POSIX-safe single-quoted shell argument.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// BuildCollectionScript appends a status-bearing end marker to the user's
// commands. The marker is printed only after all commands have finished, so a
// hang or a lost connection cannot be mistaken for successful completion.
func BuildCollectionScript(commands, marker string) string {
	body := strings.TrimRight(commands, "\r\n")
	marker = strings.TrimSpace(marker)
	return "(\n" + body + "\n)\n" +
		"__lc_status=$?\n" +
		"printf '\\n%s:%s\\n' " + ShellQuote(marker) + " \"$__lc_status\"\n" +
		"exit \"$__lc_status\"\n"
}

func BuildCollectionCommand(commands, marker string) string {
	return "sh -c " + ShellQuote(BuildCollectionScript(commands, marker))
}

func BuildArchiveCommand(tempDir, archivePath string, paths []string) string {
	quoted := make([]string, 0, len(paths))
	for _, p := range paths {
		quoted = append(quoted, ShellQuote(p))
	}
	return "mkdir -p " + ShellQuote(tempDir) +
		" && tar -czf " + ShellQuote(archivePath) + " -- " + strings.Join(quoted, " ")
}

func ExpandArchiveName(template string, combo model.Combination, role string) string {
	now := time.Now()
	timestamp := now.Format("20060102-150405")
	replacer := strings.NewReplacer(
		"{combo}", combo.Name,
		"{device}", strings.ToUpper(role),
		"{timestamp}", timestamp,
		"{date}", now.Format("20060102"),
	)
	name := replacer.Replace(template)
	name = sanitizeFilename(name)
	if name == "" {
		name = "logs_" + strings.ToUpper(role) + "_" + timestamp + ".tar.gz"
	}
	lower := strings.ToLower(name)
	if !strings.HasSuffix(lower, ".tar.gz") && !strings.HasSuffix(lower, ".tgz") {
		name += ".tar.gz"
	}
	if utf8.RuneCountInString(name) > 150 {
		ext := ".tar.gz"
		base := strings.TrimSuffix(name, ext)
		runes := []rune(base)
		if len(runes) > 140 {
			runes = runes[:140]
		}
		name = string(runes) + ext
	}
	return name
}

func SafeComboDir(name, id string) string {
	base := sanitizeFilename(name)
	if base == "" {
		base = "combo"
	}
	id = sanitizeFilename(id)
	if len(id) > 8 {
		id = id[len(id)-8:]
	}
	if id == "" {
		return base
	}
	return base + "-" + id
}

func sanitizeFilename(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		ok := unicode.IsLetter(r) || unicode.IsNumber(r) || r == '.' || r == '_' || r == '-'
		if ok {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), ".-_ ")
	if out == "" {
		out = "logs"
	}
	if isWindowsReserved(out) {
		out = "_" + out
	}
	return out
}

func isWindowsReserved(name string) bool {
	base := strings.ToUpper(strings.TrimSuffix(name, path.Ext(name)))
	switch base {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	}
	return false
}

func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n/div >= unit && exp < 4 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
