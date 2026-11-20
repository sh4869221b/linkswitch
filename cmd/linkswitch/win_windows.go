//go:build windows && (amd64 || arm64)

package main

import (
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	// Go's syscall package marks kernel32.dll as a system DLL. Resolve
	// all other GUI DLLs from the OS directory, not the launch directory.
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	systemDirectory = windowsSystemDirectory()
	user32          = syscall.NewLazyDLL(filepath.Join(systemDirectory, "user32.dll"))
	gdi32           = syscall.NewLazyDLL(filepath.Join(systemDirectory, "gdi32.dll"))
	comdlg32        = syscall.NewLazyDLL(filepath.Join(systemDirectory, "comdlg32.dll"))
	moveMemory      = kernel32.NewProc("RtlMoveMemory")

	registerClass  = user32.NewProc("RegisterClassExW")
	createWindow   = user32.NewProc("CreateWindowExW")
	defWindow      = user32.NewProc("DefWindowProcW")
	callWindow     = user32.NewProc("CallWindowProcW")
	changeProc     = user32.NewProc("SetWindowLongPtrW")
	destroyWindow  = user32.NewProc("DestroyWindow")
	showWindow     = user32.NewProc("ShowWindow")
	updateWindow   = user32.NewProc("UpdateWindow")
	moveWindow     = user32.NewProc("MoveWindow")
	setWindowPos   = user32.NewProc("SetWindowPos")
	loadIcon       = user32.NewProc("LoadIconW")
	loadCursor     = user32.NewProc("LoadCursorW")
	setFocus       = user32.NewProc("SetFocus")
	getFocus       = user32.NewProc("GetFocus")
	getKeyState    = user32.NewProc("GetKeyState")
	enableWindow   = user32.NewProc("EnableWindow")
	isEnabled      = user32.NewProc("IsWindowEnabled")
	isVisible      = user32.NewProc("IsWindowVisible")
	getMessage     = user32.NewProc("GetMessageW")
	translate      = user32.NewProc("TranslateMessage")
	dispatch       = user32.NewProc("DispatchMessageW")
	postQuit       = user32.NewProc("PostQuitMessage")
	send           = user32.NewProc("SendMessageW")
	getClient      = user32.NewProc("GetClientRect")
	getWindowText  = user32.NewProc("GetWindowTextW")
	getTextLen     = user32.NewProc("GetWindowTextLengthW")
	setWindowText  = user32.NewProc("SetWindowTextW")
	beginPaint     = user32.NewProc("BeginPaint")
	endPaint       = user32.NewProc("EndPaint")
	invalidate     = user32.NewProc("InvalidateRect")
	fillRect       = user32.NewProc("FillRect")
	drawText       = user32.NewProc("DrawTextW")
	messageBox     = user32.NewProc("MessageBoxW")
	setDPIContext  = user32.NewProc("SetProcessDpiAwarenessContext")
	setDPIAware    = user32.NewProc("SetProcessDPIAware")
	getDPI         = user32.NewProc("GetDpiForSystem")
	getWindowDPI   = user32.NewProc("GetDpiForWindow")
	getMetrics     = user32.NewProc("GetSystemMetrics")
	getSystemInfo  = user32.NewProc("SystemParametersInfoW")
	getModule      = kernel32.NewProc("GetModuleHandleW")
	openClipboard  = user32.NewProc("OpenClipboard")
	emptyClipboard = user32.NewProc("EmptyClipboard")
	setClipboard   = user32.NewProc("SetClipboardData")
	closeClipboard = user32.NewProc("CloseClipboard")
	globalAlloc    = kernel32.NewProc("GlobalAlloc")
	globalLock     = kernel32.NewProc("GlobalLock")
	globalUnlock   = kernel32.NewProc("GlobalUnlock")
	globalFree     = kernel32.NewProc("GlobalFree")

	createFont   = gdi32.NewProc("CreateFontW")
	createBrush  = gdi32.NewProc("CreateSolidBrush")
	createPen    = gdi32.NewProc("CreatePen")
	selectObject = gdi32.NewProc("SelectObject")
	deleteObject = gdi32.NewProc("DeleteObject")
	setTextColor = gdi32.NewProc("SetTextColor")
	setBkColor   = gdi32.NewProc("SetBkColor")
	setBkMode    = gdi32.NewProc("SetBkMode")
	roundRect    = gdi32.NewProc("RoundRect")
	ellipse      = gdi32.NewProc("Ellipse")
	moveTo       = gdi32.NewProc("MoveToEx")
	lineTo       = gdi32.NewProc("LineTo")
	bezier       = gdi32.NewProc("PolyBezier")
	openFileDlg  = comdlg32.NewProc("GetOpenFileNameW")
	dlgError     = comdlg32.NewProc("CommDlgExtendedError")
)

const (
	wmSize           = 0x0005
	wmDestroy        = 0x0002
	wmPaint          = 0x000f
	wmClose          = 0x0010
	wmErase          = 0x0014
	wmSetFont        = 0x0030
	wmGetMinMax      = 0x0024
	wmDrawItem       = 0x002b
	wmKeyDown        = 0x0100
	wmCommand        = 0x0111
	wmColorEdit      = 0x0133
	wmColorList      = 0x0134
	wmColorStatic    = 0x0138
	wmLbuttonDown    = 0x0201
	wmLbuttonUp      = 0x0202
	wmCaptureChanged = 0x0215
	wmCancelMode     = 0x001f
	wmDPIChanged     = 0x02e0
	wsTabStop        = 0x00010000
	wsChild          = 0x40000000
	wsVScroll        = 0x00200000
	wsClipChildren   = 0x02000000
	wsOverlapped     = 0x00cf0000
	wsExClientEdge   = 0x200
	esAutoHScroll    = 0x0080
	esAutoVScroll    = 0x0040
	esMultiline      = 0x0004
	esWantReturn     = 0x1000
	emSetLimit       = 0x00c5
	enSetFocus       = 0x0100
	enKillFocus      = 0x0200
	enChange         = 0x0300
	bsOwnerDraw      = 0x0000000b
	lbsNotify        = 0x0001
	lbsOwnerDraw     = 0x0010
	lbsHasStrings    = 0x0040
	lbsNoIntegral    = 0x0100
	lbAddString      = 0x0180
	lbResetContent   = 0x0184
	lbSetCurSel      = 0x0186
	lbGetCurSel      = 0x0188
	lbGetItemRect    = 0x0198
	lbSetItemHeight  = 0x01a0
	lbItemFromPoint  = 0x01a9
	lbnSelChange     = 1
	lbnDblClk        = 2
	odsSelected      = 0x0001
	odsDisabled      = 0x0004
	odsFocus         = 0x0010
	odsNoFocusRect   = 0x0200
	dtLeft           = 0x0000
	dtCenter         = 0x0001
	dtVCenter        = 0x0004
	dtWordBreak      = 0x0010
	dtSingleLine     = 0x0020
	dtNoPrefix       = 0x0800
	dtEllipsis       = 0x8000
	mbError          = 0x00000010
	mbQuestion       = 0x00000020
	mbWarning        = 0x00000030
	mbYesNo          = 0x00000004
	mbDefaultNo      = 0x00000100
	idYes            = 6
)

type point struct{ x, y int32 }
type rect struct{ left, top, right, bottom int32 }

type wndclass struct {
	size, style            uint32
	proc                   uintptr
	clsExtra, wndExtra     int32
	instance, icon, cursor uintptr
	background             uintptr
	menu, class            *uint16
	iconSmall              uintptr
}
type msg struct {
	hwnd           uintptr
	message        uint32
	wParam, lParam uintptr
	time           uint32
	pt             point
	private        uint32
}
type paintStruct struct {
	hdc     uintptr
	erase   int32
	rc      rect
	restore int32
	update  int32
	rgb     [32]byte
}
type drawItem struct {
	controlType, id, item, action, state uint32
	hwnd, hdc                            uintptr
	rc                                   rect
	data                                 uintptr
}
type minMaxInfo struct {
	reserved, maxSize, maxPosition, minTrackSize, maxTrackSize point
}
type openFileName struct {
	size                         uint32
	owner, instance              uintptr
	filter, customFilter         *uint16
	maxCustomFilter, filterIndex uint32
	file                         *uint16
	maxFile                      uint32
	fileTitle                    *uint16
	maxFileTitle                 uint32
	initialDir, title            *uint16
	flags                        uint32
	fileOffset, fileExtension    uint16
	defExt                       *uint16
	customData, hook             uintptr
	template                     *uint16
	reserved                     uintptr
	reservedInt, flagsEx         uint32
}

func windowsSystemDirectory() string {
	var b [32768]uint16
	p := kernel32.NewProc("GetSystemDirectoryW")
	n, _, _ := p.Call(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	if n == 0 || n >= uintptr(len(b)) {
		panic("could not resolve the Windows system directory")
	}
	dir := syscall.UTF16ToString(b[:n])
	if !filepath.IsAbs(dir) {
		panic("Windows provided a non-absolute system directory")
	}
	return dir
}

// Win32 message pointers are native addresses. Copy their contents while the
// callback owns the address without interpreting a numeric LPARAM as a Go
// pointer or retaining the native address past the callback.
func readNative[T any](address uintptr) (v T) {
	moveMemory.Call(uintptr(unsafe.Pointer(&v)), address, unsafe.Sizeof(v))
	return v
}

func writeNative[T any](address uintptr, value *T) {
	moveMemory.Call(address, uintptr(unsafe.Pointer(value)), unsafe.Sizeof(*value))
}

func wide(s string) *uint16 {
	// Win32 APIs treat the first NUL as the end of the string.
	u, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return new(uint16)
	}
	return u
}

func dlg(owner uintptr, title, s string, style uintptr) uintptr {
	t, body := wide(title), wide(s)
	result, _, _ := messageBox.Call(owner, uintptr(unsafe.Pointer(body)), uintptr(unsafe.Pointer(t)), style)
	runtime.KeepAlive(t)
	runtime.KeepAlive(body)
	return result
}

func place(h uintptr, x, y, w, height int32) {
	if h != 0 {
		moveWindow.Call(h, uintptr(x), uintptr(y), uintptr(w), uintptr(height), 1)
	}
}

func readText(hwnd uintptr) string {
	n, _, _ := getTextLen.Call(hwnd)
	b := make([]uint16, n+1)
	getWindowText.Call(hwnd, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	return syscall.UTF16ToString(b)
}

func writeText(hwnd uintptr, s string) {
	p := wide(s)
	setWindowText.Call(hwnd, uintptr(unsafe.Pointer(p)))
	runtime.KeepAlive(p)
}

func addString(hwnd uintptr, s string) {
	p := wide(s)
	send.Call(hwnd, lbAddString, 0, uintptr(unsafe.Pointer(p)))
	runtime.KeepAlive(p)
}

func selected(hwnd uintptr) int {
	v, _, _ := send.Call(hwnd, lbGetCurSel, 0, 0)
	return int(int32(v))
}

func resetItems(hwnd uintptr) { send.Call(hwnd, lbResetContent, 0, 0) }

func selectItem(hwnd uintptr, i int) { send.Call(hwnd, lbSetCurSel, uintptr(i), 0) }

func setVisible(hwnd uintptr, visible bool) {
	if visible {
		showWindow.Call(hwnd, 5)
	} else {
		showWindow.Call(hwnd, 0)
	}
}

func setEnabled(hwnd uintptr, enabled bool) {
	if enabled {
		enableWindow.Call(hwnd, 1)
	} else {
		enableWindow.Call(hwnd, 0)
	}
}

func chooseEXE(owner uintptr) (string, uint32) {
	buf := make([]uint16, 32768)
	filter, _ := syscall.UTF16FromString("アプリケーション (*.exe)")
	filter = append(filter, []uint16{'*', '.', 'e', 'x', 'e', 0, 0}...)
	title, ext := wide("ブラウザーの実行ファイルを選択"), wide("exe")
	ofn := openFileName{
		owner: owner, filter: &filter[0], filterIndex: 1,
		file: &buf[0], maxFile: uint32(len(buf)),
		title: title, defExt: ext,
		// Explorer UI, existing file/path, no current-directory change.
		flags: 0x00080000 | 0x00001000 | 0x00000800 | 0x00000008,
	}
	ofn.size = uint32(unsafe.Sizeof(ofn))
	ok, _, _ := openFileDlg.Call(uintptr(unsafe.Pointer(&ofn)))
	runtime.KeepAlive(filter)
	runtime.KeepAlive(title)
	runtime.KeepAlive(ext)
	if ok == 0 {
		code, _, _ := dlgError.Call()
		return "", uint32(code)
	}
	return syscall.UTF16ToString(buf), 0
}

func writeClipboard(owner uintptr, content string) bool {
	data, err := syscall.UTF16FromString(content)
	if err != nil {
		return false
	}
	if ok, _, _ := openClipboard.Call(owner); ok == 0 {
		return false
	}
	defer closeClipboard.Call()
	handle, _, _ := globalAlloc.Call(0x0002, uintptr(len(data)*2))
	if handle == 0 {
		return false
	}
	address, _, _ := globalLock.Call(handle)
	if address == 0 {
		globalFree.Call(handle)
		return false
	}
	moveMemory.Call(address, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)*2))
	globalUnlock.Call(handle)
	if ok, _, _ := emptyClipboard.Call(); ok == 0 {
		globalFree.Call(handle)
		return false
	}
	if token, _, _ := setClipboard.Call(13, handle); token == 0 {
		globalFree.Call(handle)
		return false
	}
	return true
}
