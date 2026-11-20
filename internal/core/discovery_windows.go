package core

import (
	"os"
	"path/filepath"
)

// DiscoverBrowsers returns known local browser executables at common
// installation paths. Portable and custom installations can be added manually.
func DiscoverBrowsers() []Browser {
	roots := []string{
		os.Getenv("ProgramW6432"),
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
		os.Getenv("LOCALAPPDATA"),
	}
	return discoverAt(roots, commonBrowsers)
}

var commonBrowsers = []browserLocation{
	{"edge", "Microsoft Edge", []string{`Microsoft\Edge\Application\msedge.exe`}},
	{"chrome", "Google Chrome", []string{`Google\Chrome\Application\chrome.exe`}},
	{"firefox", "Mozilla Firefox", []string{`Mozilla Firefox\firefox.exe`}},
	{"brave", "Brave", []string{`BraveSoftware\Brave-Browser\Application\brave.exe`}},
	{"vivaldi", "Vivaldi", []string{`Vivaldi\Application\vivaldi.exe`}},
	{"opera", "Opera", []string{`Opera\opera.exe`, `Programs\Opera\opera.exe`}},
	{"chromium", "Chromium", []string{`Chromium\Application\chrome.exe`}},
}

type browserLocation struct {
	slug, name string
	relative   []string
}

func discoverAt(roots []string, candidates []browserLocation) []Browser {
	var found []Browser
	for _, item := range candidates {
		if path := findInstalledBrowser(roots, item.relative); path != "" {
			found = append(found, Browser{ID: "discovered-" + item.slug, Name: item.name, Executable: path})
		}
	}
	return found
}

func findInstalledBrowser(roots, relative []string) string {
	for _, dir := range roots {
		if !filepath.IsAbs(dir) {
			continue
		}
		for _, sub := range relative {
			resolved, err := resolveExecutable(filepath.Join(dir, sub))
			if err == nil {
				return resolved
			}
		}
	}
	return ""
}
