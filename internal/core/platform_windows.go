package core

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	// kernel32.dll and advapi32.dll are already registered as known system
	// DLLs by the Go syscall package. Resolve shell32.dll by absolute path.
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	moveFileEx     = kernel32.NewProc("MoveFileExW")
	getSystemDir   = kernel32.NewProc("GetSystemDirectoryW")
	createMutex    = kernel32.NewProc("CreateMutexW")
	releaseMutex   = kernel32.NewProc("ReleaseMutex")
	advapi32       = syscall.NewLazyDLL("advapi32.dll")
	regCreateKeyEx = advapi32.NewProc("RegCreateKeyExW")
	regSetValueEx  = advapi32.NewProc("RegSetValueExW")
	regDeleteTree  = advapi32.NewProc("RegDeleteTreeW")
	regDeleteValue = advapi32.NewProc("RegDeleteValueW")
)

func withConfigWriteLock(path string, work func() error) error {
	hash := sha256.Sum256([]byte(strings.ToUpper(filepath.Clean(path))))
	name, _ := syscall.UTF16PtrFromString(fmt.Sprintf(`Local\LinkSwitch.Config.%x`, hash[:16]))
	handle, _, err := createMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return fmt.Errorf("設定ファイルをロックできません: %w", err)
	}
	defer syscall.CloseHandle(syscall.Handle(handle))

	// A Windows mutex must be released by the same native thread that
	// acquired it. The kernel recovers the mutex if that thread exits.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	result, waitErr := syscall.WaitForSingleObject(syscall.Handle(handle), 2000)
	if waitErr != nil {
		return fmt.Errorf("設定ファイルをロックできません: %w", waitErr)
	}
	const waitObject = 0
	const waitAbandoned = 0x80
	if result != waitObject && result != waitAbandoned {
		return errors.New("別の LinkSwitch が設定を保存中です。もう一度お試しください")
	}
	defer releaseMutex.Call(handle)
	return work()
}

func replaceFile(src, dst string) error {
	from, err := syscall.UTF16PtrFromString(src)
	if err != nil {
		return err
	}
	to, err := syscall.UTF16PtrFromString(dst)
	if err != nil {
		return err
	}
	const replaceExisting = 1
	const writeThrough = 8
	for attempt := 0; ; attempt++ {
		ok, _, err := moveFileEx.Call(uintptr(unsafe.Pointer(from)), uintptr(unsafe.Pointer(to)), replaceExisting|writeThrough)
		if ok != 0 {
			return nil
		}
		// Another copy of the picker may briefly be reading the same file.
		if attempt >= 3 || (err != syscall.Errno(32) && err != syscall.Errno(33)) {
			return err
		}
		time.Sleep(20 * time.Millisecond)
	}
}

var systemShell = sync.OnceValues(loadSystemShell)

func loadSystemShell() (*syscall.LazyDLL, error) {
	var path [1024]uint16
	n, _, err := getSystemDir.Call(uintptr(unsafe.Pointer(&path[0])), uintptr(len(path)))
	if n == 0 {
		return nil, err
	}
	if n >= uintptr(len(path)) {
		return nil, errors.New("Windows システムフォルダーのパスが長すぎます")
	}
	return syscall.NewLazyDLL(filepath.Join(syscall.UTF16ToString(path[:n]), "shell32.dll")), nil
}

func shellNotifyAssociations() {
	shell, err := systemShell()
	if err != nil {
		return
	}
	const shcneAssocChanged = 0x08000000
	const shcnfIDListFlush = 0x1000
	proc := shell.NewProc("SHChangeNotify")
	if proc.Find() == nil {
		proc.Call(shcneAssocChanged, shcnfIDListFlush, 0, 0)
	}
}

// OpenDefaultAppsSettings opens the Windows UI without changing any defaults.
func OpenDefaultAppsSettings() error {
	shell, err := systemShell()
	if err != nil {
		return fmt.Errorf("既定のアプリの設定を開けません: %w", err)
	}
	open, _ := syscall.UTF16PtrFromString("open")
	uri, _ := syscall.UTF16PtrFromString("ms-settings:defaultapps")
	proc := shell.NewProc("ShellExecuteW")
	if err := proc.Find(); err != nil {
		return fmt.Errorf("既定のアプリの設定を開けません: %w", err)
	}
	const swShowNormal = 1
	code, _, _ := proc.Call(0, uintptr(unsafe.Pointer(open)), uintptr(unsafe.Pointer(uri)), 0, 0, swShowNormal)
	if code <= 32 {
		return fmt.Errorf("既定のアプリの設定を開けません (Windows エラー %d)", code)
	}
	return nil
}

func openCurrentUserKey(path string, access uint32) (syscall.Handle, error) {
	part, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var key syscall.Handle
	err = syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, part, 0, access, &key)
	return key, err
}

func createCurrentUserKey(path string) (syscall.Handle, error) {
	part, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var key syscall.Handle
	code, _, _ := regCreateKeyEx.Call(syscall.HKEY_CURRENT_USER, uintptr(unsafe.Pointer(part)), 0, 0, 0,
		syscall.KEY_SET_VALUE, 0, uintptr(unsafe.Pointer(&key)), 0)
	if code != 0 {
		return 0, syscall.Errno(code)
	}
	return key, nil
}

func setCurrentUserString(path, name, value string) error {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	data, err := syscall.UTF16FromString(value)
	if err != nil {
		return err
	}
	key, err := createCurrentUserKey(path)
	if err != nil {
		return err
	}
	defer syscall.RegCloseKey(key)
	code, _, _ := regSetValueEx.Call(uintptr(key), uintptr(unsafe.Pointer(namePtr)), 0, syscall.REG_SZ,
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)*2))
	if code != 0 {
		return syscall.Errno(code)
	}
	return nil
}

func deleteCurrentUserValue(path, name string) error {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	key, err := openCurrentUserKey(path, syscall.KEY_SET_VALUE)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer syscall.RegCloseKey(key)
	code, _, _ := regDeleteValue.Call(uintptr(key), uintptr(unsafe.Pointer(namePtr)))
	if code != 0 && syscall.Errno(code) != syscall.ERROR_FILE_NOT_FOUND {
		return syscall.Errno(code)
	}
	return nil
}

func deleteCurrentUserTree(parent, name string) error {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	const delete = 0x00010000
	key, err := openCurrentUserKey(parent, delete|syscall.KEY_ENUMERATE_SUB_KEYS|syscall.KEY_QUERY_VALUE|syscall.KEY_SET_VALUE)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer syscall.RegCloseKey(key)
	code, _, _ := regDeleteTree.Call(uintptr(key), uintptr(unsafe.Pointer(namePtr)))
	if code != 0 && syscall.Errno(code) != syscall.ERROR_FILE_NOT_FOUND {
		return syscall.Errno(code)
	}
	return nil
}

func currentUserString(path, name string) (string, error) {
	key, err := openCurrentUserKey(path, syscall.KEY_QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer syscall.RegCloseKey(key)
	return registryString(key, name)
}

func registryString(key syscall.Handle, name string) (string, error) {
	part, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return "", err
	}
	// Only short LinkSwitch registration values are queried.
	var buf [2048]uint16
	size := uint32(len(buf) * 2)
	var valueType uint32
	if err := syscall.RegQueryValueEx(key, part, nil, &valueType, (*byte)(unsafe.Pointer(&buf[0])), &size); err != nil {
		return "", err
	}
	if valueType != syscall.REG_SZ || size < 2 || size%2 != 0 {
		return "", fmt.Errorf("%s はレジストリ文字列ではありません", name)
	}
	return syscall.UTF16ToString(buf[:size/2]), nil
}
