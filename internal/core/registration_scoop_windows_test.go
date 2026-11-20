package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistrationExecutableUsesVerifiedScoopCurrent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "a Scoop root", "ApPs", "LinkSwitch")
	version := filepath.Join(root, "1.0.0", "LinkSwitch.exe")
	current := filepath.Join(root, "current", "LinkSwitch.exe")
	for _, p := range []string{version, current} {
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(version, []byte("running version"), 0600); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(version)
	if err != nil {
		t.Fatal(err)
	}
	check := func(path, want string) {
		t.Helper()
		got, err := registrationExecutable(path)
		if err != nil || got != want {
			t.Fatalf("registrationExecutable(%q) = %q, %v; want %q", path, got, err, want)
		}
	}

	check(version, canonical)
	if err := os.WriteFile(current, []byte("another version"), 0600); err != nil {
		t.Fatal(err)
	}
	check(version, canonical)
	if err := os.Remove(current); err != nil {
		t.Fatal(err)
	}
	// A hard link models SameFile behavior without Developer Mode/admin needed
	// to create a symlink. A Scoop current junction also passes the same check.
	if err := os.Link(version, current); err != nil {
		t.Fatal(err)
	}
	check(version, current)
	check(current, current)

	unrelated := filepath.Join(t.TempDir(), "NotApps", "linkswitch")
	portable := filepath.Join(unrelated, "1.0.0", "LinkSwitch.exe")
	portableCurrent := filepath.Join(unrelated, "current", "LinkSwitch.exe")
	for _, p := range []string{portable, portableCurrent} {
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Link(version, portable); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(version, portableCurrent); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(portable)
	if err != nil {
		t.Fatal(err)
	}
	check(portable, want)
}
