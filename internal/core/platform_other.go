//go:build !windows

package core

import (
	"errors"
	"os"
	"sync"
)

func replaceFile(src, dst string) error { return os.Rename(src, dst) }

var configWriteMu sync.Mutex

func withConfigWriteLock(_ string, work func() error) error {
	configWriteMu.Lock()
	defer configWriteMu.Unlock()
	return work()
}

func DiscoverBrowsers() []Browser { return nil }

var errWindowsOnly = errors.New("この操作は Windows 専用です")

func RegisterDefaultBrowser() error   { return errWindowsOnly }
func UnregisterDefaultBrowser() error { return errWindowsOnly }
func OpenDefaultAppsSettings() error  { return errWindowsOnly }
