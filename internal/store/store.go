package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"logcollector/internal/model"
)

const (
	ConfigFileName = "logcollector.config.json"
	configEnvVar   = "LOGCOLLECTOR_CONFIG"
)

type Store struct {
	mu        sync.RWMutex
	path      string
	cfg       model.AppConfig
	loadError string
}

// Open loads the portable config next to the executable. During development or
// on read-only installs it falls back to the per-user config directory.
// LOGCOLLECTOR_CONFIG overrides discovery and is also used by tests.
func Open(explicitPath string) (*Store, error) {
	path, err := ResolvePath(explicitPath)
	if err != nil {
		return nil, err
	}
	s := &Store{path: path, cfg: model.DefaultConfig()}
	s.cfg.DownloadDir = DefaultDownloadDir()

	data, readErr := os.ReadFile(path)
	if readErr == nil {
		var cfg model.AppConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			s.loadError = fmt.Sprintf("配置文件解析失败：%v。可修正后重新保存，当前使用空白配置。", err)
		} else {
			cfg.Normalize()
			if cfg.DownloadDir == "" {
				cfg.DownloadDir = DefaultDownloadDir()
			}
			s.cfg = cfg
			if err := cfg.Validate(); err != nil {
				s.loadError = fmt.Sprintf("配置文件内容有误：%v", err)
			}
		}
	} else if !errors.Is(readErr, fs.ErrNotExist) {
		s.loadError = fmt.Sprintf("读取配置文件失败：%v", readErr)
	} else {
		// Create a portable starter file on first launch.
		if err := s.saveLocked(&s.cfg, false); err != nil {
			s.loadError = fmt.Sprintf("创建配置文件失败：%v", err)
		}
	}
	if s.cfg.DownloadDir == "" {
		s.cfg.DownloadDir = DefaultDownloadDir()
	}
	return s, nil
}

func ResolvePath(explicit string) (string, error) {
	if explicit == "" {
		explicit = os.Getenv(configEnvVar)
	}
	if explicit != "" {
		return filepath.Abs(explicit)
	}

	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)
		exeCandidate := filepath.Join(exeDir, ConfigFileName)
		userCandidate := ""
		if userDir, err := os.UserConfigDir(); err == nil {
			userCandidate = filepath.Join(userDir, "LogCollector", ConfigFileName)
		}
		// Prefer an existing file. This makes portable copies keep their data
		// even if the executable directory is moved.
		if fileExists(exeCandidate) {
			return exeCandidate, nil
		}
		if userCandidate != "" && fileExists(userCandidate) {
			return userCandidate, nil
		}
		if canWriteDir(exeDir) {
			return exeCandidate, nil
		}
		if userCandidate != "" {
			return userCandidate, nil
		}
	}
	if userDir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(userDir, "LogCollector", ConfigFileName), nil
	}
	return "", errors.New("无法确定配置文件保存位置")
}

func DefaultDownloadDir() string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return filepath.Join(home, "Downloads", "LogCollector")
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "downloads")
	}
	return filepath.Join(".", "downloads")
}

func (s *Store) Path() string { return s.path }

func (s *Store) LoadError() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadError
}

func (s *Store) Get() model.AppConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return model.Clone(s.cfg)
}

func (s *Store) Save(cfg model.AppConfig) (model.AppConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cfg.Version == 0 {
		cfg.Version = model.ConfigVersion
	}
	if strings.TrimSpace(cfg.DownloadDir) == "" {
		cfg.DownloadDir = DefaultDownloadDir()
	}
	cfg.Normalize()
	ensureIDs(&cfg)
	if err := cfg.Validate(); err != nil {
		return model.AppConfig{}, err
	}
	if err := s.saveLocked(&cfg, true); err != nil {
		return model.AppConfig{}, err
	}
	s.cfg = cfg
	s.loadError = ""
	return model.Clone(cfg), nil
}

func (s *Store) saveLocked(cfg *model.AppConfig, validate bool) error {
	if validate {
		if err := cfg.Validate(); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".logcollector-config-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()
	if err := tmp.Chmod(0o600); err != nil && os.PathSeparator != '\\' {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Windows can fail to replace an existing file if another process has it
	// open without FILE_SHARE_DELETE. Retry after removing our stale file.
	if err := os.Rename(tmpName, s.path); err != nil {
		if removeErr := os.Remove(s.path); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
			return err
		}
		if err := os.Rename(tmpName, s.path); err != nil {
			return err
		}
	}
	return nil
}

func ensureIDs(cfg *model.AppConfig) {
	for i := range cfg.Combinations {
		if strings.TrimSpace(cfg.Combinations[i].ID) == "" {
			cfg.Combinations[i].ID = newID()
		}
	}
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "combo-fallback"
	}
	return "combo-" + hex.EncodeToString(buf)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func canWriteDir(dir string) bool {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return false
	}
	f, err := os.CreateTemp(dir, ".logcollector-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}
