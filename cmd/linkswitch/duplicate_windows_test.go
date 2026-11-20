//go:build windows && (amd64 || arm64)

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unicode/utf16"
	"unsafe"

	"linkswitch/internal/core"
)

func TestDuplicateName(t *testing.T) {
	existing := []core.Browser{{Name: "brave (コピー)"}, {Name: "Brave (コピー 2) "}}
	if got := duplicateName("Brave", existing); got != "Brave (コピー 3)" {
		t.Errorf("suggested name = %q", got)
	}
	long := strings.Repeat("🦁", profileNameLimit)
	got := duplicateName(long, nil)
	if n := len(utf16.Encode([]rune(got))); n > profileNameLimit || !strings.HasSuffix(got, " (コピー)") {
		t.Errorf("long name used %d UTF-16 units or lost the suffix: %q", n, got)
	}
}

func TestDuplicateBrowserProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LOCALAPPDATA", home)
	exe := filepath.Join(home, "Brave Browser", "brave.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0700); err != nil {
		t.Fatal(err)
	}
	// Validation requires a regular .exe file; this test never launches it.
	if err := os.WriteFile(exe, []byte("browser fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	source := core.Browser{ID: "source", Name: "Brave", Executable: exe, Args: []string{"--profile-directory=Profile 1", "", "--app={url}", "末尾  "}}
	original := core.Config{Browsers: []core.Browser{{ID: "first", Name: "First", Executable: exe}, source}}
	if err := core.SaveConfig(original); err != nil {
		t.Fatal(err)
	}
	path, err := core.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	a := profileTestWindow(t, original)
	command := func(id, notification int) {
		send.Call(a.hwnd, wmCommand, uintptr(id|notification<<16), a.controls[id])
	}
	selectSource := func() {
		selectItem(a.controls[idProfiles], 1)
		command(idProfiles, lbnSelChange)
	}
	enabled := func(id int) bool {
		result, _, _ := isEnabled.Call(a.controls[id])
		return result != 0
	}
	visible := func(id int) bool {
		result, _, _ := isVisible.Call(a.controls[id])
		return result != 0
	}
	assertUnchanged := func() {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil || string(data) != string(before) || !reflect.DeepEqual(a.cfg, original) {
			t.Fatalf("original config changed before save (read error: %v): %+v", err, a.cfg)
		}
	}

	if visible(idDuplicate) {
		t.Fatal("duplicate is visible on the link picker")
	}
	command(idSettings, 0)
	selectSource()
	if !enabled(idDuplicate) || !visible(idDuplicate) || visible(idCancelDuplicate) {
		t.Fatal("selected profile does not expose the duplicate action")
	}
	command(idDuplicate, 0)
	if !a.duplicating || selected(a.controls[idProfiles]) != -1 ||
		enabled(idDuplicate) || enabled(idRemove) || enabled(idUp) || enabled(idDown) || !visible(idCancelDuplicate) {
		t.Fatal("copy draft did not enter an isolated edit state")
	}
	if name, path, args := readText(a.controls[idName]), readText(a.controls[idExecutable]), readText(a.controls[idArgs]); name != "Brave (コピー)" || path != exe || args != strings.Join(source.Args, "\r\n") {
		t.Fatalf("incorrect prefill: name %q, executable %q, args %q", name, path, args)
	}
	assertUnchanged()

	writeText(a.controls[idName], "Do not save")
	writeText(a.controls[idArgs], "--private-window")
	command(idCancelDuplicate, 0)
	if a.duplicating || a.dirty || selected(a.controls[idProfiles]) != 1 || readText(a.controls[idName]) != source.Name ||
		!enabled(idDuplicate) || visible(idCancelDuplicate) {
		t.Fatal("cancel did not restore the source profile")
	}
	assertUnchanged()

	command(idDuplicate, 0)
	command(idSave, 0)
	saved, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Browsers) != 3 || !reflect.DeepEqual(saved.Browsers[:2], original.Browsers) {
		t.Fatalf("save changed original profiles or ordering: %+v", saved)
	}
	copy := saved.Browsers[2]
	if copy.ID == "" || copy.ID == source.ID || copy.ID == "first" || copy.Name != "Brave (コピー)" ||
		copy.Executable != source.Executable || !reflect.DeepEqual(copy.Args, source.Args) {
		t.Fatalf("saved an incorrect copy: %+v", copy)
	}
	if a.duplicating || a.dirty || selected(a.controls[idProfiles]) != 2 || selected(a.controls[idChooser]) != 2 {
		t.Fatal("save did not select the appended profile in both lists")
	}

	selectSource()
	command(idDuplicate, 0)
	if got := readText(a.controls[idName]); got != "Brave (コピー 2)" {
		t.Fatalf("repeated duplication suggested %q", got)
	}
	writeText(a.controls[idName], "Brave / Private")
	writeText(a.controls[idArgs], "--incognito\r\n{url}")
	command(idSave, 0)
	repeated, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(repeated.Browsers) != 4 || !reflect.DeepEqual(repeated.Browsers[:3], saved.Browsers) {
		t.Fatalf("repeated save replaced an existing profile: %+v", repeated)
	}
	edited := repeated.Browsers[3]
	if edited.ID == "" || edited.ID == copy.ID || edited.ID == source.ID || edited.ID == "first" ||
		edited.Name != "Brave / Private" || edited.Executable != exe || !reflect.DeepEqual(edited.Args, []string{"--incognito", "{url}"}) {
		t.Fatalf("saved the wrong edited copy: %+v", edited)
	}

	selectItem(a.controls[idProfiles], 2)
	command(idProfiles, lbnSelChange)
	writeText(a.controls[idName], "Brave / App")
	command(idSave, 0)
	renamed, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	repeated.Browsers[2].Name = "Brave / App"
	if !reflect.DeepEqual(renamed, repeated) {
		t.Fatalf("renaming a copy changed its ID or other fields: %+v", renamed)
	}
}

// Use actual Win32 controls and the production WM_COMMAND handler. All window
// messages under test are synchronous and run on the controls' owning thread.
func profileTestWindow(t *testing.T, cfg core.Config) *appState {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	previousApp, previousChooser, previousProc := app, chooserWndProc, previousChooserProc
	t.Cleanup(func() {
		app, chooserWndProc, previousChooserProc = previousApp, previousChooser, previousProc
	})
	instance, _, _ := getModule.Call(0)
	a := &appState{dpi: 96, instance: instance, configOK: true, cfg: cfg, editing: -1,
		controls: map[int]uintptr{}, brushes: map[uintptr]uintptr{}}
	app = a
	class := wide("LinkSwitch.DuplicateTest")
	wc := wndclass{proc: syscall.NewCallback(windowProc), instance: instance, class: class}
	wc.size = uint32(unsafe.Sizeof(wc))
	if result, _, err := registerClass.Call(uintptr(unsafe.Pointer(&wc))); result == 0 {
		t.Fatalf("register test window: %v", err)
	}
	t.Cleanup(func() {
		user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(class)), instance)
		runtime.KeepAlive(class)
	})
	title := wide("LinkSwitch duplicate test")
	hwnd, _, err := createWindow.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), wsOverlapped|wsClipChildren,
		0, 0, 900, 560, 0, 0, instance, 0)
	runtime.KeepAlive(title)
	if hwnd == 0 {
		t.Fatalf("create test window: %v", err)
	}
	a.hwnd = hwnd
	t.Cleanup(func() {
		destroyWindow.Call(hwnd)
		a.releasePaintObjects()
	})
	a.initControls()
	a.refreshLists(0, -1)
	a.fillForm(-1)
	a.refreshPage()
	showWindow.Call(hwnd, 4) // SW_SHOWNOACTIVATE
	return a
}
