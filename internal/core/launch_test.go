package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValidateURL(t *testing.T) {
	for _, raw := range []string{
		"http://localhost:8080/",
		"https://[::1]/",
		"https://example.test/a%2Fb?q=a%20b&x=%22quoted%22#part",
		"HTTPS://example.test/日本語?q=検索",
		"https://example.test/search?q=$(literal);foo&value=%5C",
		"https://例.example/",
	} {
		if err := ValidateURL(raw); err != nil {
			t.Errorf("ValidateURL(%q): %v", raw, err)
		}
	}
	for _, raw := range []string{
		"", "www.example.test", "//example.test", "https:example.test", "https:///path", "https://:8080",
		"file:///C:/secret.txt", "javascript:alert(1)", "mailto:test@example.test",
		"--incognito", " https://example.test/", "https://example.test/　", "https://example.test/a b",
		"https://example.test/\t", "https://example.test/\r\n", "https://example.test/\x00",
		"https://example.test/\xff", "https://example.test/%zz",
		"https://user@example.test", "https://user:secret@example.test",
		`https://example.test/" --user-data-dir=evil`, `https://fake.example\@other.test/`,
	} {
		if err := ValidateURL(raw); err == nil {
			t.Errorf("ValidateURL(%q) = nil; want error", raw)
		}
	}
	const secret = "VerySecret_DoNotEcho"
	if err := ValidateURL("https://user:" + secret + "@example.test/"); err == nil || strings.Contains(err.Error(), secret) {
		t.Errorf("error must not include URL credentials: %v", err)
	}
}

func TestBrowserArgs(t *testing.T) {
	const raw = "https://example.test/a%20b?q=%22quoted%22&x=$(not-a-shell)#a"
	for _, tc := range []struct {
		name string
		tmpl []string
		want []string
	}{
		{"default", nil, []string{raw}},
		{"append", []string{"--profile-directory=Profile 1", "--incognito"}, []string{"--profile-directory=Profile 1", "--incognito", raw}},
		{"insert", []string{"-P", "個人 用", "{url}", "--new-tab"}, []string{"-P", "個人 用", raw, "--new-tab"}},
		{"attached", []string{"--url={url}"}, []string{"--url=" + raw}},
		{"slash", []string{"/open:{url}"}, []string{"/open:" + raw}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := append([]string(nil), tc.tmpl...)
			got, err := browserArgs(tc.tmpl, raw)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("browserArgs(%q) = %q, %v; want %q", tc.tmpl, got, err, tc.want)
			}
			if !reflect.DeepEqual(tc.tmpl, original) {
				t.Errorf("template unexpectedly modified: %q", tc.tmpl)
			}
		})
	}

	for _, tmpl := range [][]string{
		{"{url}", "--x={url}"}, {"prefix{url}"}, {`"{url}"`}, {"--option {url}"}, {"--option={url}suffix"},
		{"--foo=\x00bar"}, {"a\r\nb"}, {"\xff"},
	} {
		if got, err := browserArgs(tmpl, raw); err == nil {
			t.Errorf("browserArgs(%q) = %q; want error", tmpl, got)
		}
	}
}

func TestDrivePathRejectsSpecialWindowsPaths(t *testing.T) {
	for _, path := range []string{`C:\Program Files\browser.exe`, `d:/apps/firefox.exe`} {
		if !isDrivePath(path) {
			t.Errorf("isDrivePath(%q) = false", path)
		}
	}
	for _, path := range []string{`\\server\share\browser.exe`, `\\?\C:\browser.exe`, `\dir\browser.exe`, `C:browser.exe`, `C:\a:alternate.exe`, `C:\wild?.exe`, `0:\browser.exe`} {
		if isDrivePath(path) {
			t.Errorf("isDrivePath(%q) = true", path)
		}
	}
}

func TestResolveExecutable(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"CMD.exe", "explorer.exe", "RUNDLL32.EXE", "powershell.exe", "pwsh.exe"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("not a browser"), 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := resolveExecutable(path); err == nil || !strings.Contains(err.Error(), "ブラウザー本体") {
			t.Errorf("blocked executable %s: %v", name, err)
		}
	}
	for _, path := range []string{"browser.exe", filepath.Join(dir, "browser.cmd"), filepath.Join(dir, "missing.exe")} {
		if _, err := resolveExecutable(path); err == nil {
			t.Errorf("unexpected acceptance of %q", path)
		}
	}
	directory := filepath.Join(dir, "directory.exe")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveExecutable(directory); err == nil {
		t.Error("accepted directory as executable")
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	self := filepath.Join(dir, "renamed.exe")
	if err := os.Link(exe, self); err != nil {
		t.Skipf("hard link not supported: %s", err)
	}
	if _, err := resolveExecutable(self); err == nil || !strings.Contains(err.Error(), "自身") {
		t.Errorf("self hard link: %v", err)
	}
}

// Running a copy of the test executable allows this test to check argv at the
// real process boundary. It does not launch cmd.exe, a browser, or a network request.
func TestLaunchPreservesRealArgv(t *testing.T) {
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(dir, "ブラウザー テスト.exe")
	if err := os.WriteFile(copyPath, content, 0700); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(dir, "args.json")
	t.Setenv("LINKSWITCH_TEST_RESULT", result)
	raw := "https://example.test/P%2FQ?x=$(literal)&v=%22quoted%22#日本語"
	browser := Browser{
		ID: "test", Name: "作業", Executable: copyPath,
		Args: []string{"-test.run=^TestLaunchHelper$", "--", "--profile-directory=Profile 1", "a \"quoted\" arg", "C:\\a b\\", "{url}"},
	}
	cmd, err := browserCommand(browser, raw)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("launch helper: %v, %s", err, out)
	}
	data, err := os.ReadFile(result)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	want := []string{"--profile-directory=Profile 1", "a \"quoted\" arg", "C:\\a b\\", raw}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("child received %q; expected %q", got, want)
	}
}

func TestLaunchHelper(t *testing.T) {
	result := os.Getenv("LINKSWITCH_TEST_RESULT")
	if result == "" {
		return
	}
	for i, a := range os.Args {
		if a == "--" {
			data, err := json.Marshal(os.Args[i+1:])
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(result, data, 0600); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("missing --")
}
