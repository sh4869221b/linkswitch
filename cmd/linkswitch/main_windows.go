//go:build windows && (amd64 || arm64)

package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"linkswitch/internal/core"
)

const (
	idOpen = 100 + iota
	idSettings
	idDefaults
	idURL
	idCopy
	idChooser
	idCancel
	idProfiles
	idNew
	idDiscover
	idName
	idExecutable
	idBrowse
	idArgs
	idSave
	idRemove
	idUp
	idDown
	idUnregister
	idDuplicate
	idCancelDuplicate
)

const (
	pageOpen = iota
	pageSettings
)

const profileNameLimit = 200 // UTF-16 code units in the native edit control.

type appState struct {
	hwnd, instance uintptr
	dpi            int32
	page           int
	controls       map[int]uintptr
	cfg            core.Config
	configOK       bool
	editing        int // Source profile while duplicating; otherwise the profile being edited.
	duplicating    bool
	dirty          bool
	filling        bool
	status         string
	statusWarn     bool
	formLeft       int32
	formWidth      int32
	footerTop      int32
	fonts          [numFonts]uintptr
	brushes        map[uintptr]uintptr
}

var app *appState
var chooserWndProc uintptr
var previousChooserProc uintptr
var pressedArrow = -1

func arguments(args []string) (string, bool) {
	switch len(args) {
	case 0:
		return "", true
	case 1:
		return args[0], core.ValidateURL(args[0]) == nil
	case 2:
		if args[0] == "--url" {
			return args[1], core.ValidateURL(args[1]) == nil
		}
	}
	return "", false
}

func main() {
	runtime.LockOSThread()

	url, ok := arguments(os.Args[1:])
	if !ok {
		dlg(0, "LinkSwitch", "開けるのは有効な http:// または https:// のリンクだけです。\n手動で起動する場合: LinkSwitch.exe --url URL", mbError)
		return
	}

	if setDPIContext.Find() == nil {
		if result, _, _ := setDPIContext.Call(^uintptr(3)); result == 0 {
			setDPIAware.Call()
		}
	} else {
		setDPIAware.Call()
	}
	dpi := int32(96)
	if getDPI.Find() == nil {
		if d, _, _ := getDPI.Call(); d > 0 {
			dpi = int32(d)
		}
	}
	inst, _, _ := getModule.Call(0)
	app = &appState{dpi: dpi, instance: inst, controls: map[int]uintptr{}, brushes: map[uintptr]uintptr{}, editing: -1}
	config, configErr := core.LoadConfig()
	app.cfg, app.configOK = config, configErr == nil
	if len(config.Browsers) == 0 && url == "" {
		app.page = pageSettings
	}
	if configErr != nil {
		app.status, app.statusWarn = "設定ファイルを読み込めません。修正してから再起動してください。", true
	}

	cls := wide("LinkSwitch.Main")
	icon, _, _ := loadIcon.Call(inst, 101)
	if icon == 0 {
		icon, _, _ = loadIcon.Call(0, 32512)
	}
	cursor, _, _ := loadCursor.Call(0, 32512)
	wc := wndclass{
		style: 3, proc: syscall.NewCallback(windowProc), instance: inst,
		class: cls, cursor: cursor, icon: icon, iconSmall: icon,
	}
	wc.size = uint32(unsafe.Sizeof(wc))
	if registered, _, _ := registerClass.Call(uintptr(unsafe.Pointer(&wc))); registered == 0 {
		dlg(0, "LinkSwitch", "ウィンドウを準備できませんでした。", mbError)
		return
	}
	var work rect
	gotWork, _, _ := getSystemInfo.Call(0x0030, 0, uintptr(unsafe.Pointer(&work)), 0)
	if gotWork == 0 {
		sw, _, _ := getMetrics.Call(0)
		sh, _, _ := getMetrics.Call(1)
		work.right, work.bottom = int32(sw), int32(sh)
	}
	w := min(app.s(1110), work.right-work.left-app.s(12))
	h := min(app.s(660), work.bottom-work.top-app.s(12))
	x, y := work.left+(work.right-work.left-w)/2, work.top+(work.bottom-work.top-h)/2
	title := wide("LinkSwitch")
	hwnd, _, _ := createWindow.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)), wsOverlapped|wsClipChildren,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0, 0, inst, 0)
	runtime.KeepAlive(cls)
	runtime.KeepAlive(title)
	if hwnd == 0 {
		dlg(0, "LinkSwitch", "ウィンドウを開けませんでした。", mbError)
		return
	}

	app.hwnd = hwnd
	defer app.releasePaintObjects()
	if getWindowDPI.Find() == nil {
		if d, _, _ := getWindowDPI.Call(hwnd); d != 0 {
			app.dpi = int32(d)
		}
	}
	app.initControls()
	writeText(app.controls[idURL], url)
	app.refreshLists(0, -1)
	app.fillForm(-1)
	app.refreshPage()
	showWindow.Call(hwnd, 5)
	updateWindow.Call(hwnd)
	if configErr != nil {
		where, _ := core.ConfigPath()
		dlg(hwnd, "設定ファイルの読み込みエラー", app.status+"\n\n"+where+"\n\n"+configErr.Error(), mbError)
	}
	if app.page == pageOpen {
		if url != "" && len(app.cfg.Browsers) != 0 {
			setFocus.Call(app.controls[idChooser])
		} else if url == "" {
			setFocus.Call(app.controls[idURL])
		} else {
			setFocus.Call(app.controls[idSettings])
		}
	} else {
		setFocus.Call(app.controls[idDiscover])
	}
	var m msg
	for {
		result, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(result) <= 0 {
			break
		}
		if m.message == wmKeyDown && app.key(m.hwnd, m.wParam) {
			continue
		}
		translate.Call(uintptr(unsafe.Pointer(&m)))
		dispatch.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (a *appState) s(x int32) int32 { return (x*a.dpi + 48) / 96 }

func (a *appState) child(id int, class, label string, style, ex uintptr) {
	c, l := wide(class), wide(label)
	h, _, _ := createWindow.Call(ex, uintptr(unsafe.Pointer(c)), uintptr(unsafe.Pointer(l)), wsChild|wsTabStop|style,
		0, 0, 1, 1, a.hwnd, uintptr(id), a.instance, 0)
	a.controls[id] = h
	runtime.KeepAlive(c)
	runtime.KeepAlive(l)
}

func (a *appState) button(id int, label string) { a.child(id, "BUTTON", label, bsOwnerDraw, 0) }

func (a *appState) initControls() {
	a.button(idOpen, "リンクを開く")
	a.button(idSettings, "ブラウザー設定")
	a.button(idDefaults, "既定ブラウザー設定")
	a.child(idURL, "EDIT", "", esAutoHScroll, 0)
	a.button(idCopy, "コピー")
	listStyle := uintptr(wsVScroll | lbsNotify | lbsOwnerDraw | lbsHasStrings | lbsNoIntegral)
	a.child(idChooser, "LISTBOX", "", listStyle, 0)
	chooserWndProc = syscall.NewCallback(chooserProc)
	previousChooserProc, _, _ = changeProc.Call(a.controls[idChooser], ^uintptr(3), chooserWndProc)
	a.button(idCancel, "キャンセル")
	a.child(idProfiles, "LISTBOX", "", listStyle, 0)
	a.button(idNew, "新規")
	a.button(idDiscover, "ブラウザーを検出")
	a.child(idName, "EDIT", "", esAutoHScroll, wsExClientEdge)
	a.child(idExecutable, "EDIT", "", esAutoHScroll, wsExClientEdge)
	a.button(idBrowse, "参照…")
	a.child(idArgs, "EDIT", "", esMultiline|esAutoVScroll|esWantReturn|wsVScroll, wsExClientEdge)
	a.button(idSave, "保存")
	a.button(idRemove, "削除")
	a.button(idUp, "上へ")
	a.button(idDown, "下へ")
	a.button(idUnregister, "既定候補から解除")
	a.button(idDuplicate, "複製")
	a.button(idCancelDuplicate, "キャンセル")
	for _, limit := range []struct{ id, max int }{{idURL, 32766}, {idName, profileNameLimit}, {idExecutable, 32766}, {idArgs, 16384}} {
		send.Call(a.controls[limit.id], emSetLimit, uintptr(limit.max), 0)
	}
	// Native edit cue banner strings; not persisted.
	for _, cue := range []struct {
		id    int
		label string
	}{
		{idURL, "https://example.com/"},
		{idName, "例：Microsoft Edge / 仕事"},
		{idExecutable, "ブラウザーの .exe ファイル"},
	} {
		c := wide(cue.label)
		send.Call(a.controls[cue.id], 0x1501, 0, uintptr(unsafe.Pointer(c)))
		runtime.KeepAlive(c)
	}
	a.makeFonts()
}

var openControls = []int{idURL, idCopy, idChooser, idCancel}
var settingsControls = []int{idNew, idDiscover, idProfiles, idUp, idDown, idDuplicate, idName, idExecutable, idBrowse, idArgs, idRemove, idCancelDuplicate, idSave, idUnregister}

func (a *appState) refreshPage() {
	for _, id := range openControls {
		setVisible(a.controls[id], a.page == pageOpen && (id != idChooser || len(a.cfg.Browsers) > 0))
	}
	for _, id := range settingsControls {
		if id == idRemove || id == idCancelDuplicate {
			continue // These share the same space; updateControls chooses which to show.
		}
		setVisible(a.controls[id], a.page == pageSettings && (id != idProfiles || len(a.cfg.Browsers) > 0))
	}
	for _, id := range []int{idOpen, idSettings, idDefaults} {
		setVisible(a.controls[id], true)
		invalidate.Call(a.controls[id], 0, 0)
	}
	a.updateControls()
	a.layout()
	invalidate.Call(a.hwnd, 0, 1)
}

func (a *appState) updateControls() {
	hasURL := strings.TrimSpace(readText(a.controls[idURL])) != ""
	setEnabled(a.controls[idCopy], hasURL)
	setEnabled(a.controls[idChooser], len(a.cfg.Browsers) > 0)
	setEnabled(a.controls[idProfiles], len(a.cfg.Browsers) > 0)
	for _, id := range []int{idNew, idDiscover, idName, idExecutable, idBrowse, idArgs, idSave} {
		setEnabled(a.controls[id], a.configOK)
	}
	hasSelection := a.configOK && !a.duplicating && a.editing >= 0 && a.editing < len(a.cfg.Browsers)
	setEnabled(a.controls[idDuplicate], hasSelection)
	setEnabled(a.controls[idRemove], hasSelection)
	setEnabled(a.controls[idUp], hasSelection && a.editing > 0)
	setEnabled(a.controls[idDown], hasSelection && a.editing < len(a.cfg.Browsers)-1)
	setVisible(a.controls[idRemove], a.page == pageSettings && !a.duplicating)
	setVisible(a.controls[idCancelDuplicate], a.page == pageSettings && a.duplicating)
}

func (a *appState) refreshLists(choose, settings int) {
	for _, id := range []int{idChooser, idProfiles} {
		h := a.controls[id]
		resetItems(h)
		for _, b := range a.cfg.Browsers {
			addString(h, b.Name+" / "+profileLabel(b))
		}
	}
	if choose >= 0 && choose < len(a.cfg.Browsers) {
		selectItem(a.controls[idChooser], choose)
	}
	if settings >= 0 && settings < len(a.cfg.Browsers) {
		selectItem(a.controls[idProfiles], settings)
	}
}

func (a *appState) notify(s string, warning bool) {
	a.status, a.statusWarn = s, warning
	invalidate.Call(a.hwnd, 0, 0)
}

func (a *appState) switchPage(page int) {
	if a.page == page {
		return
	}
	if !a.confirmDiscard() {
		return
	}
	if a.dirty {
		if a.duplicating {
			selectItem(a.controls[idProfiles], a.editing)
		}
		a.fillForm(a.editing)
	}
	a.page = page
	if a.configOK {
		a.status, a.statusWarn = "", false
	}
	a.refreshPage()
	if page == pageSettings {
		if len(a.cfg.Browsers) == 0 {
			if a.configOK {
				setFocus.Call(a.controls[idDiscover])
			}
		} else {
			if a.editing == -1 {
				selectItem(a.controls[idProfiles], 0)
				a.fillForm(0)
			}
			setFocus.Call(a.controls[idProfiles])
		}
	} else if readText(a.controls[idURL]) == "" {
		setFocus.Call(a.controls[idURL])
	} else if len(a.cfg.Browsers) != 0 {
		setFocus.Call(a.controls[idChooser])
	}
}

func (a *appState) close() {
	if a.confirmDiscard() {
		destroyWindow.Call(a.hwnd)
	}
}

func (a *appState) key(h, key uintptr) bool {
	switch key {
	case 'L':
		ctrl, _, _ := getKeyState.Call(0x11)
		if ctrl&0x8000 != 0 {
			a.switchPage(pageOpen)
			if a.page == pageOpen {
				setFocus.Call(a.controls[idURL])
				send.Call(a.controls[idURL], 0x00b1, 0, ^uintptr(0))
			}
			return true
		}
	case 0x1b: // Escape
		a.close()
		return true
	case 0x09: // Tab
		order := []int{idOpen, idSettings, idDefaults}
		if a.page == pageOpen {
			order = append(order, openControls...)
		} else {
			order = append(order, settingsControls...)
		}
		shift, _, _ := getKeyState.Call(0x10)
		step := 1
		if shift&0x8000 != 0 {
			step = -1
		}
		at := -1
		for i, id := range order {
			if h == a.controls[id] {
				at = i
				break
			}
		}
		if at == -1 && step < 0 {
			at = 0
		}
		for range order {
			at = (at + step + len(order)) % len(order)
			next := a.controls[order[at]]
			visible, _, _ := isVisible.Call(next)
			enabled, _, _ := isEnabled.Call(next)
			if visible != 0 && enabled != 0 {
				setFocus.Call(next)
				break
			}
		}
		return true
	case 0x0d: // Enter
		if h == a.controls[idArgs] {
			return false
		}
		if h == a.controls[idName] || h == a.controls[idExecutable] {
			a.save()
			return true
		}
		if h == a.controls[idURL] || h == a.controls[idChooser] {
			a.launchSelected()
			return true
		}
		if h == a.controls[idProfiles] {
			setFocus.Call(a.controls[idName])
			return true
		}
		for id, control := range a.controls {
			if h == control {
				a.click(id)
				return true
			}
		}
	}
	return false
}

func (a *appState) click(id int) {
	switch id {
	case idOpen:
		a.switchPage(pageOpen)
	case idSettings:
		a.switchPage(pageSettings)
	case idDefaults:
		a.defaults()
	case idCopy:
		if s := readText(a.controls[idURL]); s != "" {
			if writeClipboard(a.hwnd, s) {
				a.notify("リンクをコピーしました。", false)
			} else {
				a.notify("コピーできませんでした。もう一度お試しください。", true)
			}
		}
	case idCancel:
		a.close()
	case idNew:
		if a.confirmDiscard() {
			selectItem(a.controls[idProfiles], -1)
			a.fillForm(-1)
			setFocus.Call(a.controls[idName])
			a.notify("表示名と実行ファイルを入力して保存してください。", false)
		}
	case idDiscover:
		a.discover()
	case idBrowse:
		if path, code := chooseEXE(a.hwnd); code != 0 {
			a.notify(fmt.Sprintf("ファイル選択を開けませんでした（エラー 0x%X）。パスを直接入力できます。", code), true)
		} else if path != "" {
			writeText(a.controls[idExecutable], path)
		}
	case idSave:
		a.save()
	case idDuplicate:
		a.duplicate()
	case idCancelDuplicate:
		a.cancelDuplicate()
	case idRemove:
		a.remove()
	case idUp:
		a.reorder(-1)
	case idDown:
		a.reorder(1)
	case idUnregister:
		a.unregister()
	}
}

func (a *appState) launchSelected() {
	s := strings.TrimSpace(readText(a.controls[idURL]))
	if core.ValidateURL(s) != nil {
		a.notify("有効な http:// または https:// のリンクを入力してください。", true)
		setFocus.Call(a.controls[idURL])
		return
	}
	i := selected(a.controls[idChooser])
	if i < 0 || i >= len(a.cfg.Browsers) {
		a.notify("先に「ブラウザー設定」でブラウザーを登録してください。", true)
		setFocus.Call(a.controls[idSettings])
		return
	}
	b := a.cfg.Browsers[i]
	if err := core.Launch(b, s); err != nil {
		a.notify("起動できませんでした。「ブラウザー設定」で実行ファイルと引数を確認してください。", true)
		return
	}
	destroyWindow.Call(a.hwnd)
}

func (a *appState) confirmDiscard() bool {
	return !a.dirty || dlg(a.hwnd, "未保存の変更", "保存していない変更を破棄しますか？", mbYesNo|mbWarning|mbDefaultNo) == idYes
}

func (a *appState) fillForm(index int) {
	a.editing = index
	a.duplicating = false
	var b core.Browser
	if index >= 0 && index < len(a.cfg.Browsers) {
		b = a.cfg.Browsers[index]
	}
	a.writeForm(b, false)
}

func (a *appState) writeForm(b core.Browser, dirty bool) {
	a.filling = true
	writeText(a.controls[idName], b.Name)
	writeText(a.controls[idExecutable], b.Executable)
	writeText(a.controls[idArgs], strings.Join(b.Args, "\r\n"))
	a.filling, a.dirty = false, dirty
	a.updateControls()
	invalidate.Call(a.hwnd, 0, 0)
}

func (a *appState) duplicate() {
	if a.page != pageSettings || !a.configOK || a.duplicating || a.editing < 0 || a.editing >= len(a.cfg.Browsers) || !a.confirmDiscard() {
		return
	}
	b := a.cfg.Browsers[a.editing]
	b.Name = duplicateName(b.Name, a.cfg.Browsers)
	a.duplicating = true
	selectItem(a.controls[idProfiles], -1)
	a.writeForm(b, true)
	setFocus.Call(a.controls[idName])
	send.Call(a.controls[idName], 0x00b1, 0, ^uintptr(0)) // Select the suggested name for easy replacement.
	a.notify("複製した設定を確認し、別の設定として保存してください。", false)
}

func duplicateName(name string, browsers []core.Browser) string {
	used := make(map[string]bool, len(browsers))
	for _, b := range browsers {
		used[strings.ToLower(strings.TrimSpace(b.Name))] = true
	}
	name = strings.TrimSpace(name)
	for i := 1; ; i++ {
		suffix := " (コピー)"
		if i > 1 {
			suffix = " (コピー " + strconv.Itoa(i) + ")"
		}
		budget := profileNameLimit - len([]rune(suffix)) // Every rune in the suffix is one UTF-16 code unit.
		prefix := name
		for at, r := range name {
			budget--
			if r > 0xffff { // Supplementary characters take a UTF-16 surrogate pair.
				budget--
			}
			if budget < 0 {
				prefix = strings.TrimSpace(name[:at])
				break
			}
		}
		candidate := prefix + suffix
		if !used[strings.ToLower(candidate)] {
			return candidate
		}
	}
}

func (a *appState) cancelDuplicate() {
	if !a.duplicating {
		return
	}
	selectItem(a.controls[idProfiles], a.editing)
	a.fillForm(a.editing)
	setFocus.Call(a.controls[idProfiles])
	a.notify("", false)
}

func (a *appState) store(browsers []core.Browser, at int, success string) bool {
	c := core.Config{Browsers: browsers}
	if err := core.SaveConfigIfUnchanged(a.cfg, c); err != nil {
		if errors.Is(err, core.ErrConfigChanged) {
			dlg(a.hwnd, "別のウィンドウで設定が更新されました", "ほかの LinkSwitch で保存された設定を上書きしませんでした。最新の設定を読み込むには、このウィンドウを閉じて LinkSwitch を起動し直してください。", mbWarning)
		} else {
			dlg(a.hwnd, "設定の保存エラー", "設定を保存できませんでした。設定フォルダーの権限と空き容量を確認してください。\n\n"+err.Error(), mbError)
		}
		return false
	}
	a.cfg = c
	a.refreshPage()
	choice := at
	if choice < 0 {
		choice = 0
	}
	a.refreshLists(choice, at)
	a.fillForm(at)
	a.notify(success, false)
	return true
}

func (a *appState) save() {
	if !a.configOK {
		return
	}
	b := core.Browser{Name: strings.TrimSpace(readText(a.controls[idName])), Executable: strings.TrimSpace(readText(a.controls[idExecutable]))}
	at := a.editing
	argsText := strings.ReplaceAll(readText(a.controls[idArgs]), "\r\n", "\n")
	if at >= 0 && argsText == strings.Join(a.cfg.Browsers[at].Args, "\n") {
		// An existing file can contain explicit empty arguments. Preserve the
		// exact argv when the argument text hasn't changed.
		b.Args = slices.Clone(a.cfg.Browsers[at].Args)
	} else {
		for _, arg := range strings.Split(argsText, "\n") {
			if arg != "" {
				b.Args = append(b.Args, arg)
			}
		}
	}
	if a.duplicating {
		at = -1
	}
	if at >= 0 {
		b.ID = a.cfg.Browsers[at].ID
	} else {
		for {
			id, err := core.NewBrowserID()
			if err != nil {
				a.notify("ブラウザーIDを作成できませんでした。", true)
				return
			}
			if !slices.ContainsFunc(a.cfg.Browsers, func(b core.Browser) bool { return b.ID == id }) {
				b.ID = id
				break
			}
		}
	}
	if err := core.ValidateBrowser(b); err != nil {
		dlg(a.hwnd, "入力内容を確認", "表示名、実行ファイル、起動引数を確認してください。\n\n"+err.Error(), mbError)
		return
	}
	next := append([]core.Browser(nil), a.cfg.Browsers...)
	success := "ブラウザー設定を保存しました。"
	if a.duplicating {
		success = "ブラウザー設定を複製しました。"
	}
	if at < 0 {
		at = len(next)
		next = append(next, b)
	} else {
		next[at] = b
	}
	a.store(next, at, success)
}

func (a *appState) remove() {
	if !a.configOK || a.duplicating || a.editing < 0 {
		return
	}
	if dlg(a.hwnd, "ブラウザー設定の削除", "このブラウザー設定を一覧から削除しますか？\nブラウザー本体やプロファイルのデータは削除しません。", mbYesNo|mbWarning|mbDefaultNo) != idYes {
		return
	}
	i := a.editing
	next := append([]core.Browser(nil), a.cfg.Browsers[:i]...)
	next = append(next, a.cfg.Browsers[i+1:]...)
	a.store(next, min(i, len(next)-1), "ブラウザー設定を削除しました。")
}

func (a *appState) reorder(delta int) {
	from, to := a.editing, a.editing+delta
	if !a.configOK || a.duplicating || from < 0 || to < 0 || to >= len(a.cfg.Browsers) || !a.confirmDiscard() {
		return
	}
	next := append([]core.Browser(nil), a.cfg.Browsers...)
	next[from], next[to] = next[to], next[from]
	a.store(next, to, "表示順を変更しました。")
}

func (a *appState) discover() {
	if !a.configOK || !a.confirmDiscard() {
		return
	}
	discovered := core.DiscoverBrowsers()
	next := append([]core.Browser(nil), a.cfg.Browsers...)
	for _, b := range discovered {
		exists := false
		for _, present := range next {
			if strings.EqualFold(present.Executable, b.Executable) && len(present.Args) == 0 {
				exists = true
				break
			}
		}
		if !exists {
			id, err := core.NewBrowserID()
			if err != nil {
				a.notify("ブラウザーIDを作成できませんでした。", true)
				return
			}
			b.ID = id
			next = append(next, b)
		}
	}
	added := len(next) - len(a.cfg.Browsers)
	if added == 0 {
		a.notify("新しいブラウザーは見つかりませんでした。「新規」から手動で追加できます。", false)
		return
	}
	a.store(next, len(a.cfg.Browsers), fmt.Sprintf("ブラウザーを %d 件追加しました。", added))
}

func (a *appState) defaults() {
	body := "LinkSwitch を既定のブラウザー候補として登録し、Windows の［既定のアプリ］を開きます。\n\nWindows の画面で LinkSwitch を選び、HTTP と HTTPS の既定を設定してください。この操作だけで既定は変更されません。"
	if dlg(a.hwnd, "既定ブラウザー設定", body, mbYesNo|mbQuestion|mbDefaultNo) != idYes {
		return
	}
	if err := core.RegisterDefaultBrowser(); err != nil {
		dlg(a.hwnd, "登録できませんでした", "LinkSwitch を既定候補として登録できませんでした。\n\n"+err.Error(), mbError)
		return
	}
	if err := core.OpenDefaultAppsSettings(); err != nil {
		dlg(a.hwnd, "登録できました", "Windows の設定画面を開けませんでした。［設定］→［アプリ］→［既定のアプリ］から LinkSwitch を選んでください。", mbWarning)
		return
	}
	a.notify("Windows の設定で LinkSwitch を選択してください。", false)
}

func (a *appState) unregister() {
	body := "LinkSwitch が現在の既定なら、先に Windows の［既定のアプリ］で別のブラウザーを選んでください。\n\nLinkSwitch 自身の登録だけを解除します。今すぐ解除しますか？"
	if dlg(a.hwnd, "既定候補から解除", body, mbYesNo|mbWarning|mbDefaultNo) != idYes {
		return
	}
	if err := core.UnregisterDefaultBrowser(); err != nil {
		dlg(a.hwnd, "解除できませんでした", "LinkSwitch の登録を解除できませんでした。\n\n"+err.Error(), mbError)
		return
	}
	a.notify("既定のブラウザー候補から LinkSwitch を解除しました。", false)
}

func (a *appState) hitArrow(hwnd, l uintptr) int {
	at, _, _ := send.Call(hwnd, lbItemFromPoint, 0, l)
	index := int(uint16(at))
	x, y := int32(int16(l)), int32(int16(l>>16))
	var r rect
	if at>>16 == 0 && index < len(a.cfg.Browsers) {
		if result, _, _ := send.Call(hwnd, lbGetItemRect, uintptr(index), uintptr(unsafe.Pointer(&r))); int32(result) != -1 && a.arrowRect(r).contains(x, y) {
			return index
		}
	}
	return -1
}

func chooserProc(hwnd uintptr, message uint32, w, l uintptr) uintptr {
	// A click must begin and end on the same arrow. Elsewhere, the native
	// listbox selects the item; double click and Enter also open it.
	if app != nil && previousChooserProc != 0 {
		switch message {
		case wmLbuttonDown:
			pressedArrow = app.hitArrow(hwnd, l)
		case wmLbuttonUp:
			index := pressedArrow
			pressedArrow = -1
			if index >= 0 && app.hitArrow(hwnd, l) == index {
				prior, _, _ := callWindow.Call(previousChooserProc, hwnd, uintptr(message), w, l)
				selectItem(hwnd, index)
				app.launchSelected()
				return prior
			}
		case wmCaptureChanged, wmCancelMode:
			pressedArrow = -1
		}
	}
	if previousChooserProc != 0 {
		v, _, _ := callWindow.Call(previousChooserProc, hwnd, uintptr(message), w, l)
		return v
	}
	v, _, _ := defWindow.Call(hwnd, uintptr(message), w, l)
	return v
}

func windowProc(hwnd uintptr, message uint32, w, l uintptr) uintptr {
	a := app
	if a == nil {
		v, _, _ := defWindow.Call(hwnd, uintptr(message), w, l)
		return v
	}
	switch message {
	case wmErase:
		return 1
	case wmClose:
		a.close()
		return 0
	case wmDestroy:
		postQuit.Call(0)
		return 0
	case wmGetMinMax:
		if l != 0 {
			info := readNative[minMaxInfo](l)
			info.minTrackSize = point{x: a.s(750), y: a.s(422)}
			writeNative(l, &info)
		}
		return 0
	case wmSize:
		a.layout()
		return 0
	case wmDPIChanged:
		if l == 0 || uint16(w) == 0 {
			return 0
		}
		a.dpi = int32(uint16(w))
		a.makeFonts()
		r := readNative[rect](l)
		setWindowPos.Call(hwnd, 0, uintptr(r.left), uintptr(r.top), uintptr(r.right-r.left), uintptr(r.bottom-r.top), 0x0004|0x0010)
		a.layout()
		invalidate.Call(hwnd, 0, 1)
		return 0
	case wmPaint:
		var p paintStruct
		dc, _, _ := beginPaint.Call(hwnd, uintptr(unsafe.Pointer(&p)))
		a.paint(dc)
		endPaint.Call(hwnd, uintptr(unsafe.Pointer(&p)))
		return 0
	case wmColorEdit:
		if l == a.controls[idURL] {
			setTextColor.Call(w, ink)
			setBkColor.Call(w, linkBackground)
			return a.brush(linkBackground)
		}
		setTextColor.Call(w, ink)
		setBkColor.Call(w, white)
		return a.brush(white)
	case wmColorStatic:
		if l == a.controls[idURL] {
			setTextColor.Call(w, ink)
			setBkColor.Call(w, linkBackground)
			return a.brush(linkBackground)
		}
	case wmColorList:
		setTextColor.Call(w, ink)
		setBkColor.Call(w, surface)
		return a.brush(surface)
	case wmDrawItem:
		if l != 0 {
			di := readNative[drawItem](l)
			a.drawControl(&di)
			return 1
		}
	case wmCommand:
		id, code := int(uint16(w)), int(uint16(w>>16))
		switch {
		case id == idURL && (code == enSetFocus || code == enKillFocus):
			invalidate.Call(hwnd, 0, 0)
		case id == idURL && code == enChange:
			a.updateControls()
			if a.statusWarn && a.configOK {
				a.notify("", false)
			}
		case (id == idName || id == idExecutable || id == idArgs) && code == enChange:
			if !a.filling && !a.dirty {
				a.dirty = true
				a.notify("", false)
			}
		case id == idChooser && code == lbnDblClk:
			a.launchSelected()
		case id == idProfiles && code == lbnSelChange:
			index := selected(a.controls[idProfiles])
			if a.duplicating || index != a.editing {
				if a.confirmDiscard() {
					a.fillForm(index)
					if a.configOK {
						a.notify("", false)
					}
				} else {
					if a.duplicating {
						selectItem(a.controls[idProfiles], -1)
					} else {
						selectItem(a.controls[idProfiles], a.editing)
					}
				}
			}
		case code == 0:
			a.click(id)
		}
		return 0
	}
	v, _, _ := defWindow.Call(hwnd, uintptr(message), w, l)
	return v
}
