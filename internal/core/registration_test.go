package core

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserRegistrationOnlyClaimsHTTPAndHTTPS(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "Link Switch 日本語", "LinkSwitch.exe")
	values, err := browserRegistration(exe)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	associations := map[string]string{}
	for _, entry := range values {
		key := entry.path + "|" + entry.name
		if _, exists := seen[key]; exists {
			t.Fatalf("duplicate value: %s", key)
		}
		seen[key] = entry.value
		if strings.Contains(strings.ToLower(entry.path), "userchoice") {
			t.Fatalf("registration must not change the chosen handler: %s", entry.path)
		}
		if entry.path == capabilitiesKey+`\URLAssociations` {
			associations[entry.name] = entry.value
		}
	}
	if len(associations) != 2 || associations["http"] != progIDHTTP || associations["https"] != progIDHTTPS {
		t.Fatalf("unexpected protocol registrations: %v", associations)
	}
	if got := seen[registeredAppsKey+"|"+appName]; got != capabilitiesKey {
		t.Errorf("RegisteredApplications = %q", got)
	}
	if got := seen[capabilitiesKey+"|ApplicationName"]; got != appName {
		t.Errorf("ApplicationName = %q", got)
	}
	want := `"` + exe + `" --url "%1"`
	for _, id := range associations {
		if got := seen[protocolClassesKey+`\`+id+`\shell\open\command|`]; got != want {
			t.Errorf("open command = %q; want %q", got, want)
		}
		if _, exists := seen[protocolClassesKey+`\`+id+"|URL Protocol"]; !exists {
			t.Errorf("missing URL Protocol marker for %s", id)
		}
	}
	if got := values[len(values)-1].path; got != registeredAppsKey {
		t.Errorf("capability published too early: last key %q", got)
	}
}

func TestBrowserRegistrationRejectsAmbiguousPaths(t *testing.T) {
	for _, path := range []string{
		"LinkSwitch.exe",
		filepath.Join(t.TempDir(), "Has %1", "LinkSwitch.exe"),
		filepath.Join(t.TempDir(), `Has a " quote`, "LinkSwitch.exe"),
	} {
		if _, err := browserRegistration(path); err == nil {
			t.Errorf("accepted ambiguous registry command path %q", path)
		}
	}
}
