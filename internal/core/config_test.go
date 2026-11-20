package core

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConfigRoundTripAndReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "LinkSwitch", "config.json")
	missing, err := loadConfig(path)
	if err != nil || len(missing.Browsers) != 0 {
		t.Fatalf("missing config: %#v, %v", missing, err)
	}
	sharedExe := filepath.Join(dir, "ブラウザー directory", "chrome.exe") // Need not exist to preserve an old profile.
	initial := Config{Browsers: []Browser{
		{ID: "work", Name: "仕事", Executable: sharedExe, Args: []string{"--profile-directory=Profile 1", "{url}"}},
		{ID: "private", Name: "プライベート", Executable: sharedExe, Args: []string{"--incognito"}},
	}}
	if err := saveConfig(path, initial); err != nil {
		t.Fatal(err)
	}
	got, err := loadConfig(path)
	if err != nil || !reflect.DeepEqual(got, initial) {
		t.Fatalf("load first config = %#v, %v; want %#v", got, err, initial)
	}

	invalid := Config{Browsers: []Browser{initial.Browsers[0], initial.Browsers[0]}}
	if err := saveConfig(path, invalid); err == nil {
		t.Fatal("saved config with duplicate IDs")
	}
	got, err = loadConfig(path)
	if err != nil || !reflect.DeepEqual(got, initial) {
		t.Fatalf("failed save modified old config: %#v, %v", got, err)
	}

	empty := Config{Browsers: []Browser{}}
	if err := saveConfig(path, empty); err != nil {
		t.Fatal(err)
	}
	got, err = loadConfig(path)
	if err != nil || !reflect.DeepEqual(got, empty) {
		t.Fatalf("load replacement = %#v, %v; want %#v", got, err, empty)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 || entries[0].Name() != "config.json" {
		t.Fatalf("unexpected files remaining after replace: %v, %v", entries, err)
	}
}

func TestConfigDoesNotHideCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	for _, raw := range []string{
		`{"version":1,"browsers":[`,
		`{"version":2,"browsers":[]}`,
		`{"version":1,"browsers":[]} {}`,
		`{"version":1,"broesers":[]}`,
		`{"version":1,"browsers":[{"id":"a","name":"x","executable":"chrome.exe"}]}`,
		strings.Repeat(" ", maxConfigSize+1),
		"{\"version\":1,\"browsers\":[],\"\xff\":0}",
	} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadConfig(path); err == nil {
			t.Errorf("accepted invalid config: %.100q", raw)
		}
	}
}

func TestStaleProfileSaveDoesNotOverwriteAnotherWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "LinkSwitch", "config.json")
	results := make(chan error, 2)
	start := make(chan struct{})
	for _, name := range []string{"設定 1", "設定 2"} {
		go func(name string) {
			<-start
			next := Config{Browsers: []Browser{{ID: "one", Name: name, Executable: filepath.Join(filepath.Dir(path), "browser.exe")}}}
			results <- saveConfigIfUnchanged(path, Config{}, next)
		}(name)
	}
	close(start)
	var wins, conflicts int
	for range 2 {
		switch err := <-results; {
		case err == nil:
			wins++
		case errors.Is(err, ErrConfigChanged):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	current, err := loadConfig(path)
	if err != nil || len(current.Browsers) != 1 || wins != 1 || conflicts != 1 {
		t.Fatalf("want one win and one conflict: wins=%d, conflicts=%d, config=%#v, err=%v", wins, conflicts, current, err)
	}
	if err := saveConfigIfUnchanged(path, current, Config{}); err != nil {
		t.Fatal(err)
	}
}

func TestNewBrowserID(t *testing.T) {
	seen := map[string]bool{}
	for range 50 {
		id, err := NewBrowserID()
		if err != nil || len(id) != 32 || seen[id] {
			t.Fatalf("NewBrowserID = %q, %v", id, err)
		}
		seen[id] = true
	}
}

func TestBrowserFieldValidation(t *testing.T) {
	good := Browser{ID: "example", Name: "Edge - 仕事", Executable: filepath.Join(t.TempDir(), "msedge.exe")}
	if err := validateBrowserFields(good); err != nil {
		t.Fatal(err)
	}
	for _, b := range []Browser{
		{ID: "", Name: good.Name, Executable: good.Executable},
		{ID: "has space", Name: good.Name, Executable: good.Executable},
		{ID: good.ID, Name: "　", Executable: good.Executable},
		{ID: good.ID, Name: "One\nTwo", Executable: good.Executable},
		{ID: good.ID, Name: good.Name, Executable: "msedge.exe"},
		{ID: good.ID, Name: good.Name, Executable: good.Executable, Args: []string{"{url}", "{url}"}},
	} {
		if err := validateBrowserFields(b); err == nil {
			t.Errorf("accepted invalid profile: %+v", b)
		}
	}
}
