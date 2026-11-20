// Package core implements LinkSwitch's profile storage and browser launching.
package core

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	configVersion = 1
	maxConfigSize = 1024 * 1024
)

// Browser is one launch profile. More than one profile can use the same executable.
type Browser struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Executable string   `json:"executable"`
	Args       []string `json:"args,omitempty"`
}

type Config struct {
	Browsers []Browser
}

type configFile struct {
	Version  int       `json:"version"`
	Browsers []Browser `json:"browsers"`
}

// ConfigPath returns the current user's settings file.
func ConfigPath() (string, error) {
	var base string
	if runtime.GOOS == "windows" {
		base = os.Getenv("LOCALAPPDATA")
	} else {
		// Useful for running the same storage tests on the host.
		var err error
		base, err = os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("設定フォルダーを特定できません: %w", err)
		}
	}
	if !filepath.IsAbs(base) {
		return "", errors.New("ユーザーの設定フォルダーを特定できません")
	}
	return filepath.Join(base, "LinkSwitch", "config.json"), nil
}

// NewBrowserID makes an opaque stable identifier for a new profile.
func NewBrowserID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("プロファイルIDを作れません: %w", err)
	}
	return hex.EncodeToString(random[:]), nil
}

func LoadConfig() (Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, err
	}
	return loadConfig(path)
}

func loadConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("設定ファイルを開けません: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxConfigSize+1))
	closeErr := f.Close()
	if err != nil {
		return Config{}, fmt.Errorf("設定ファイルを読めません: %w", err)
	}
	if closeErr != nil {
		return Config{}, fmt.Errorf("設定ファイルを閉じられません: %w", closeErr)
	}
	if len(data) > maxConfigSize {
		return Config{}, errors.New("設定ファイルが大きすぎます (上限 1 MiB)")
	}
	if !utf8.Valid(data) {
		return Config{}, errors.New("設定ファイルが UTF-8 ではありません")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var file configFile
	if err := decoder.Decode(&file); err != nil {
		return Config{}, fmt.Errorf("設定ファイルの形式が正しくありません: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return Config{}, errors.New("設定ファイルの末尾に余分なデータがあります")
	}
	if file.Version != configVersion {
		return Config{}, fmt.Errorf("未対応の設定ファイルのバージョンです: %d", file.Version)
	}
	cfg := Config{Browsers: file.Browsers}
	if err := validateConfig(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func SaveConfig(cfg Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	return withConfigWriteLock(path, func() error { return saveConfig(path, cfg) })
}

// ErrConfigChanged is returned when another instance saved a different
// configuration after the caller last loaded it.
var ErrConfigChanged = errors.New("別の LinkSwitch で設定が更新されました。設定画面を開き直してください")

// SaveConfigIfUnchanged avoids overwriting updates from another picker window.
func SaveConfigIfUnchanged(previous, next Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	return saveConfigIfUnchanged(path, previous, next)
}

func saveConfigIfUnchanged(path string, previous, next Config) error {
	return withConfigWriteLock(path, func() error {
		current, err := loadConfig(path)
		if err != nil {
			return err
		}
		if !slices.EqualFunc(current.Browsers, previous.Browsers, func(a, b Browser) bool {
			return a.ID == b.ID && a.Name == b.Name && a.Executable == b.Executable && slices.Equal(a.Args, b.Args)
		}) {
			return ErrConfigChanged
		}
		return saveConfig(path, next)
	})
}

func saveConfig(path string, cfg Config) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	browsers := cfg.Browsers
	if browsers == nil {
		browsers = []Browser{}
	}
	data, err := json.MarshalIndent(configFile{Version: configVersion, Browsers: browsers}, "", "  ")
	if err != nil {
		return fmt.Errorf("設定を変換できません: %w", err)
	}
	data = append(data, '\n')
	if len(data) > maxConfigSize {
		return errors.New("設定ファイルが大きすぎます (上限 1 MiB)")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("設定フォルダーを作れません: %w", err)
	}
	f, err := os.CreateTemp(dir, ".linkswitch-*.tmp")
	if err != nil {
		return fmt.Errorf("設定の一時ファイルを作れません: %w", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("設定ファイルを書き込めません: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("設定ファイルを同期できません: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("設定ファイルを閉じられません: %w", err)
	}
	if err := replaceFile(f.Name(), path); err != nil {
		return fmt.Errorf("設定ファイルを置き換えられません: %w", err)
	}
	return nil
}

func validateConfig(cfg Config) error {
	ids := make(map[string]bool, len(cfg.Browsers))
	for i, b := range cfg.Browsers {
		if err := validateBrowserFields(b); err != nil {
			return fmt.Errorf("設定のプロファイル %d: %w", i+1, err)
		}
		if ids[b.ID] {
			return fmt.Errorf("設定のプロファイル %d: ID が重複しています", i+1)
		}
		ids[b.ID] = true
	}
	return nil
}

// ValidateBrowser checks profile fields and the selected file. The file is
// checked again immediately before launching; saving/loading unrelated
// profiles remains possible even when a previously selected file is gone.
func ValidateBrowser(b Browser) error {
	if err := validateBrowserFields(b); err != nil {
		return err
	}
	_, err := resolveExecutable(b.Executable)
	return err
}

func validateBrowserFields(b Browser) error {
	if b.ID == "" || len(b.ID) > 100 || !utf8.ValidString(b.ID) ||
		strings.ContainsFunc(b.ID, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) {
		return errors.New("プロファイルの ID が正しくありません")
	}
	if strings.TrimSpace(b.Name) == "" || !utf8.ValidString(b.Name) ||
		strings.ContainsFunc(b.Name, unicode.IsControl) {
		return errors.New("プロファイル名を入力してください (改行や制御文字は不可)")
	}
	if err := validateExecutablePath(b.Executable); err != nil {
		return err
	}
	return validateArguments(b.Args)
}
