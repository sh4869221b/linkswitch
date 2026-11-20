package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func verifyRegistrationName() error {
	value, err := currentUserString(registeredAppsKey, appName)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case err != nil:
		return err
	case !strings.EqualFold(value, capabilitiesKey):
		return errors.New("別のアプリが同じ登録名を使用しています")
	default:
		return nil
	}
}

// checkRegistrationTree distinguishes a missing key from a key owned by
// another app. A matching registry name by itself is never proof of ownership.
func checkRegistrationTree(path string) (bool, error) {
	key, err := openCurrentUserKey(path, syscall.KEY_QUERY_VALUE)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer syscall.RegCloseKey(key)
	token, err := registryString(key, ownershipValue)
	if err == nil && token == ownershipToken {
		return true, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return false, fmt.Errorf("別のアプリが同じ登録キーを使用しています (%s)", path)
}

func checkBrowserRegistration() error {
	if err := verifyRegistrationName(); err != nil {
		return err
	}
	for _, entry := range registrationTrees {
		if _, err := checkRegistrationTree(entry[0] + `\` + entry[1]); err != nil {
			return err
		}
	}
	return nil
}

// registrationExecutable preserves Scoop's verified current junction instead
// of linking the registry to a version directory that Scoop can clean up.
func registrationExecutable(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}

	appDir := filepath.Dir(filepath.Dir(path))
	if !filepath.IsAbs(path) ||
		!strings.EqualFold(filepath.Base(path), "LinkSwitch.exe") ||
		!strings.EqualFold(filepath.Base(appDir), "linkswitch") ||
		!strings.EqualFold(filepath.Base(filepath.Dir(appDir)), "apps") {
		return resolved, nil
	}
	current := filepath.Join(appDir, "current", "LinkSwitch.exe")
	running, runningErr := os.Stat(resolved)
	candidate, candidateErr := os.Stat(current)
	if runningErr == nil && candidateErr == nil &&
		candidate.Mode().IsRegular() && os.SameFile(running, candidate) {
		return current, nil
	}
	return resolved, nil
}

// RegisterDefaultBrowser registers this executable as a possible HTTP(S)
// handler for the current user. It does not assign it as the default.
func RegisterDefaultBrowser() error {
	path, err := os.Executable()
	if err != nil {
		return fmt.Errorf("LinkSwitch 本体を確認できません: %w", err)
	}
	path, err = registrationExecutable(path)
	if err != nil {
		return fmt.Errorf("LinkSwitch 本体を確認できません: %w", err)
	}
	values, err := browserRegistration(path)
	if err != nil {
		return err
	}
	if err := checkBrowserRegistration(); err != nil {
		return fmt.Errorf("ブラウザーを登録できません: %w", err)
	}
	for _, entry := range values {
		if err := setCurrentUserString(entry.path, entry.name, entry.value); err != nil {
			return fmt.Errorf("ブラウザーを登録できません (%s): %w", entry.path, err)
		}
	}
	shellNotifyAssociations()
	return nil
}

// UnregisterDefaultBrowser removes only LinkSwitch's registration. It does
// not touch the user's choice; select a different default in Windows first.
func UnregisterDefaultBrowser() error {
	if err := checkBrowserRegistration(); err != nil {
		return fmt.Errorf("ブラウザー登録を解除できません: %w", err)
	}
	var errs []error
	if err := deleteCurrentUserValue(registeredAppsKey, appName); err != nil {
		errs = append(errs, err)
	}
	for _, entry := range registrationTrees {
		owned, err := checkRegistrationTree(entry[0] + `\` + entry[1])
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !owned {
			continue
		}
		if err := deleteCurrentUserTree(entry[0], entry[1]); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", entry[1], err))
		}
	}
	shellNotifyAssociations()
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("ブラウザー登録を解除できません: %w", err)
	}
	return nil
}
