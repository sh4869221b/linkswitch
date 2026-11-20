package core

import (
	"errors"
	"os"
	"testing"
)

// Touch only a uniquely named disposable key, never RegisteredApplications,
// Software\Classes, UserChoice, or a real application registration.
func TestRegistryHelpersInDisposableCurrentUserKey(t *testing.T) {
	id, err := NewBrowserID()
	if err != nil {
		t.Fatal(err)
	}
	const parent = "Software"
	leaf := "LinkSwitch Tests." + id
	path := parent + `\` + leaf
	t.Cleanup(func() {
		if err := deleteCurrentUserTree(parent, leaf); err != nil {
			t.Error(err)
		}
	})
	if owned, err := checkRegistrationTree(path); owned || err != nil {
		t.Fatalf("nonexistent key appears owned or invalid: %v, %v", owned, err)
	}

	const otherValue = "別アプリ"
	if err := setCurrentUserString(path, "", otherValue); err != nil {
		t.Fatal(err)
	}
	if got, err := currentUserString(path, ""); err != nil || got != otherValue {
		t.Fatalf("default value: %q, %v", got, err)
	}
	if owned, err := checkRegistrationTree(path); owned || err == nil {
		t.Fatalf("unmarked key did not detect conflict: %v, %v", owned, err)
	}
	if err := setCurrentUserString(path, ownershipValue, "another-product"); err != nil {
		t.Fatal(err)
	}
	if owned, err := checkRegistrationTree(path); owned || err == nil {
		t.Fatalf("foreign marker did not detect conflict: %v, %v", owned, err)
	}
	if err := setCurrentUserString(path, ownershipValue, ownershipToken); err != nil {
		t.Fatal(err)
	}
	if owned, err := checkRegistrationTree(path); !owned || err != nil {
		t.Fatalf("owned key not recognized: %v, %v", owned, err)
	}

	child := path + `\Child`
	if err := setCurrentUserString(child, "description", "リンク選択"); err != nil {
		t.Fatal(err)
	}
	if err := setCurrentUserString(child, "other", "Keep this value"); err != nil {
		t.Fatal(err)
	}
	if err := deleteCurrentUserValue(child, "description"); err != nil {
		t.Fatal(err)
	}
	if _, err := currentUserString(child, "description"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted value still present: %v", err)
	}
	if got, err := currentUserString(child, "other"); err != nil || got != "Keep this value" {
		t.Fatalf("deleting one value changed an unrelated value: %q, %v", got, err)
	}
	if err := deleteCurrentUserTree(parent, leaf); err != nil {
		t.Fatal(err)
	}
	if owned, err := checkRegistrationTree(path); owned || err != nil {
		t.Fatalf("deleted tree still present: %v, %v", owned, err)
	}
}
