package core

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"
)

const placeholder = "{url}"

var attachedPlaceholder = regexp.MustCompile(`^(--?[A-Za-z0-9][A-Za-z0-9._-]*=|/[A-Za-z0-9][A-Za-z0-9._-]*[:=])\{url\}$`)

// ValidateURL accepts absolute HTTP(S) links with a host. Launch passes the
// original URL, without decoding or normalizing queries, paths, or fragments.
func ValidateURL(raw string) error {
	if raw == "" || !utf8.ValidString(raw) ||
		strings.ContainsFunc(raw, func(r rune) bool {
			return r == '"' || r == '\\' || unicode.IsControl(r) || unicode.IsSpace(r)
		}) {
		return errors.New("URL に使用できない文字が含まれています")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.Hostname() == "" ||
		(!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		// Do not echo an incoming URL: its query may contain a secret.
		return errors.New("http:// または https:// で始まる有効な URL を指定してください")
	}
	if parsed.User != nil {
		return errors.New("ユーザー名やパスワードを含む URL は開けません")
	}
	return nil
}

func validateArguments(args []string) error {
	usesURL := false
	for _, arg := range args {
		if !utf8.ValidString(arg) || strings.ContainsAny(arg, "\x00\r\n") {
			return errors.New("起動引数には改行や NUL を含められません (1 行につき 1 引数)")
		}
		if !strings.Contains(arg, placeholder) {
			continue
		}
		if arg != placeholder && !attachedPlaceholder.MatchString(arg) {
			return errors.New("{url} は単独の引数か、--option={url} の形式で指定してください")
		}
		if usesURL {
			return errors.New("{url} は 1 か所だけ指定してください")
		}
		usesURL = true
	}
	return nil
}

func browserArgs(template []string, raw string) ([]string, error) {
	if err := ValidateURL(raw); err != nil {
		return nil, err
	}
	if err := validateArguments(template); err != nil {
		return nil, err
	}
	args := make([]string, 0, len(template)+1)
	inserted := false
	for _, arg := range template {
		if strings.HasSuffix(arg, placeholder) {
			arg = strings.TrimSuffix(arg, placeholder) + raw
			inserted = true
		}
		args = append(args, arg)
	}
	if !inserted {
		args = append(args, raw)
	}
	return args, nil
}

func validateExecutablePath(path string) error {
	if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".exe") ||
		!utf8.ValidString(path) || strings.ContainsAny(path, `"<>|`) ||
		strings.ContainsFunc(path, unicode.IsControl) {
		return errors.New("実行ファイルは .exe の絶対パスで指定してください")
	}
	if runtime.GOOS == "windows" && !isDrivePath(path) {
		return errors.New("実行ファイルはローカルドライブ上の通常の .exe を指定してください")
	}
	return nil
}

func isDrivePath(path string) bool {
	if len(path) < 3 || path[1] != ':' || (path[2] != '\\' && path[2] != '/') || strings.ContainsAny(path[3:], `:*?`) {
		return false
	}
	c := path[0]
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

func blockedExecutable(name string) bool {
	switch strings.ToLower(name) {
	case "cmd.exe", "command.com", "powershell.exe", "pwsh.exe", "powershell_ise.exe",
		"wscript.exe", "cscript.exe", "mshta.exe", "rundll32.exe", "regsvr32.exe",
		"explorer.exe", "openwith.exe", "shellexperiencehost.exe", "msedge_proxy.exe",
		"sh.exe", "bash.exe", "dash.exe", "zsh.exe", "fish.exe", "wsl.exe", "wt.exe",
		"python.exe", "pythonw.exe", "py.exe", "pyw.exe", "node.exe", "deno.exe", "bun.exe",
		"ruby.exe", "perl.exe", "php.exe", "java.exe", "javaw.exe", "dotnet.exe":
		return true
	default:
		return false
	}
}

func resolveExecutable(path string) (string, error) {
	if err := validateExecutablePath(path); err != nil {
		return "", err
	}
	if blockedExecutable(filepath.Base(path)) {
		return "", errors.New("ブラウザー本体を指定してください (シェルや起動仲介アプリは不可)")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("実行ファイルが見つかりません: %w", err)
	}
	if err := validateExecutablePath(resolved); err != nil {
		return "", err
	}
	if blockedExecutable(filepath.Base(resolved)) {
		return "", errors.New("ブラウザー本体を指定してください (シェルや起動仲介アプリは不可)")
	}
	file, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("実行ファイルを確認できません: %w", err)
	}
	if !file.Mode().IsRegular() {
		return "", errors.New("通常の .exe ファイルを選択してください")
	}

	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("LinkSwitch 本体を確認できません: %w", err)
	}
	if strings.EqualFold(filepath.Clean(self), filepath.Clean(resolved)) {
		return "", errors.New("LinkSwitch 自身を起動先に指定できません")
	}
	if ownFile, err := os.Stat(self); err == nil && os.SameFile(file, ownFile) {
		return "", errors.New("LinkSwitch 自身を起動先に指定できません")
	}
	return resolved, nil
}

// Launch invokes the chosen executable directly, without a shell or a URL
// association lookup. The configured executable and arguments are trusted
// user input; only the URL can be supplied by an outside caller.
func Launch(b Browser, raw string) error {
	cmd, err := browserCommand(b, raw)
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ブラウザーを起動できません: %w", err)
	}
	// The browser may remain open well after the chooser exits.
	_ = cmd.Process.Release()
	return nil
}

func browserCommand(b Browser, raw string) (*exec.Cmd, error) {
	args, err := browserArgs(b.Args, raw)
	if err != nil {
		return nil, err
	}
	path, err := resolveExecutable(b.Executable)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(path, args...)
	// The inherited working directory can be set by the application invoking
	// the URL handler. Do not pass that directory on to the browser.
	cmd.Dir = filepath.Dir(path)
	return cmd, nil
}
