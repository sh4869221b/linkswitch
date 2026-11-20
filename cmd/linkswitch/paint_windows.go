//go:build windows && (amd64 || arm64)

package main

import (
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"linkswitch/internal/core"
)

const (
	ink            = 0x381d10 // #101D38
	muted          = 0x74594a
	surface        = 0xf8f3ef
	rail           = 0xf5eee8
	selectedRail   = 0xfbe1cf
	linkBackground = 0xfcead9
	linkBorder     = 0xf4c198
	accent         = 0xe65f15
	accentDark     = 0xba460d
	white          = 0xffffff
	edge           = 0xc7b8ab
	divider        = 0xe2d7cc
	hoverWhite     = 0xfcf7f0
	disabledFill   = 0xf4efeb
	disabledInk    = 0x948579
	warning        = 0x1d257e
	orange         = 0x0863e6
	red            = 0x2f2cce
	teal           = 0x85880a
	purple         = 0xaf4a74
	rowSelected    = 0xfff3e6
)

const (
	fontBrand = iota
	fontTitle
	fontCompactTitle
	fontSection
	fontButton
	fontBody
	fontSmall
	fontURL
	fontRow
	fontBadge
	numFonts
)

func (a *appState) brush(color uintptr) uintptr {
	if b := a.brushes[color]; b != 0 {
		return b
	}
	b, _, _ := createBrush.Call(color)
	a.brushes[color] = b
	return b
}

func (a *appState) releasePaintObjects() {
	for i, font := range a.fonts {
		if font != 0 {
			deleteObject.Call(font)
			a.fonts[i] = 0
		}
	}
	for color, brush := range a.brushes {
		deleteObject.Call(brush)
		delete(a.brushes, color)
	}
}

func (a *appState) makeFonts() {
	old := a.fonts
	face := wide("Yu Gothic UI")
	for i, f := range [...]struct{ size, weight int32 }{
		{25, 700}, {31, 700}, {21, 700}, {17, 700}, {15, 600}, {15, 400}, {13, 400}, {20, 500}, {21, 700}, {27, 700},
	} {
		h, _, _ := createFont.Call(uintptr(int64(-a.s(f.size))), 0, 0, 0, uintptr(f.weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
		a.fonts[i] = h
	}
	runtime.KeepAlive(face)
	for id, h := range a.controls {
		f := fontButton
		switch id {
		case idURL:
			f = fontURL
		case idName, idExecutable, idArgs:
			f = fontBody
		}
		if h != 0 {
			send.Call(h, wmSetFont, a.fonts[f], 1)
		}
	}
	send.Call(a.controls[idChooser], lbSetItemHeight, 0, uintptr(a.s(91)))
	send.Call(a.controls[idProfiles], lbSetItemHeight, 0, uintptr(a.s(52)))
	for _, f := range old {
		if f != 0 {
			deleteObject.Call(f)
		}
	}
}

func (a *appState) dimensions() (r rect, railWidth, left, right int32) {
	getClient.Call(a.hwnd, uintptr(unsafe.Pointer(&r)))
	railWidth = a.s(225)
	pad := a.s(31)
	if r.right < a.s(940) {
		railWidth = a.s(169)
		pad = a.s(20)
	}
	return r, railWidth, railWidth + pad, r.right - pad
}

type settingsLayout struct {
	listActions, list     int32
	nameLabel, name       int32
	exeLabel, exe         int32
	argsLabel, argsHelp   int32
	args, editHeight      int32
	saveGap, buttonHeight int32
}

func settingsGeometry(compact bool) settingsLayout {
	if compact {
		return settingsLayout{
			listActions: 93, list: 133, nameLabel: 96, name: 119,
			exeLabel: 148, exe: 172, argsLabel: 202, argsHelp: 222,
			args: 245, editHeight: 27, saveGap: 43, buttonHeight: 32,
		}
	}
	return settingsLayout{
		listActions: 111, list: 156, nameLabel: 113, name: 143,
		exeLabel: 175, exe: 205, argsLabel: 239, argsHelp: 261,
		args: 287, editHeight: 31, saveGap: 47, buttonHeight: 35,
	}
}

func (a *appState) layout() {
	if a.hwnd == 0 || len(a.controls) == 0 {
		return
	}
	r, railWidth, left, right := a.dimensions()
	if r.right == 0 || r.bottom == 0 {
		return
	}
	compact := r.bottom < a.s(444)
	footerHeight, footerButtonTop, footerButtonHeight := int32(62), int32(11), int32(39)
	if compact {
		footerHeight, footerButtonTop, footerButtonHeight = 54, 9, 35
	}
	a.footerTop = r.bottom - a.s(footerHeight)
	p := func(id int, x, y, w, h int32) {
		place(a.controls[id], x, y, max(w, a.s(1)), max(h, a.s(1)))
	}
	p(idOpen, a.s(11), a.s(88), railWidth-a.s(22), a.s(46))
	p(idSettings, a.s(11), a.s(140), railWidth-a.s(22), a.s(46))
	defaultWidth := a.s(201)
	if r.right < a.s(940) {
		defaultWidth = a.s(174)
	}
	p(idDefaults, right-defaultWidth, a.s(29), defaultWidth, a.s(41))

	p(idURL, left+a.s(58), a.s(132), right-left-a.s(190), a.s(33))
	p(idCopy, right-a.s(120), a.s(125), a.s(104), a.s(39))
	p(idChooser, left-a.s(2), a.s(201), right-left+a.s(2), a.footerTop-a.s(213))
	p(idCancel, right-a.s(120), a.footerTop+a.s(footerButtonTop), a.s(120), a.s(footerButtonHeight))

	g := settingsGeometry(compact)
	mainWidth := right - left
	lw := min(a.s(293), mainWidth*40/100)
	a.formLeft, a.formWidth = left+lw+a.s(23), mainWidth-lw-a.s(23)
	saveTop := a.footerTop - a.s(g.saveGap)
	p(idProfiles, left, a.s(g.list), lw, saveTop-a.s(g.list+16))
	p(idUp, left, saveTop, a.s(66), a.s(g.buttonHeight))
	p(idDown, left+a.s(73), saveTop, a.s(66), a.s(g.buttonHeight))
	p(idNew, left, a.s(g.listActions), a.s(54), a.s(34))
	p(idDiscover, left+a.s(60), a.s(g.listActions), lw-a.s(60), a.s(34))
	fx, fw := a.formLeft, a.formWidth
	p(idName, fx, a.s(g.name), fw, a.s(g.editHeight))
	p(idExecutable, fx, a.s(g.exe), fw-a.s(66), a.s(g.editHeight))
	p(idBrowse, right-a.s(60), a.s(g.exe-1), a.s(60), a.s(g.editHeight+2))
	p(idArgs, fx, a.s(g.args), fw, max(a.s(22), saveTop-a.s(g.args+9)))
	p(idRemove, right-a.s(148), saveTop, a.s(67), a.s(g.buttonHeight))
	p(idSave, right-a.s(72), saveTop, a.s(72), a.s(g.buttonHeight))
	p(idUnregister, right-a.s(164), a.footerTop+a.s(footerButtonTop), a.s(164), a.s(footerButtonHeight))
}

func (a *appState) rect(x, y, w, h int32) rect { return rect{x, y, x + w, y + h} }

func (r rect) contains(x, y int32) bool {
	return x >= r.left && x < r.right && y >= r.top && y < r.bottom
}

func (r rect) inset(x, y int32) rect {
	return rect{r.left + x, r.top + y, r.right - x, r.bottom - y}
}

func (a *appState) fill(dc uintptr, r rect, color uintptr) {
	fillRect.Call(dc, uintptr(unsafe.Pointer(&r)), a.brush(color))
}

func (a *appState) roundShape(dc uintptr, r rect, radius int32, fill, edge uintptr) {
	if r.right <= r.left || r.bottom <= r.top {
		return
	}
	oldB, _, _ := selectObject.Call(dc, a.brush(fill))
	p, _, _ := createPen.Call(0, 1, edge)
	oldP, _, _ := selectObject.Call(dc, p)
	roundRect.Call(dc, uintptr(r.left), uintptr(r.top), uintptr(r.right), uintptr(r.bottom), uintptr(radius), uintptr(radius))
	selectObject.Call(dc, oldP)
	selectObject.Call(dc, oldB)
	deleteObject.Call(p)
}

func (a *appState) disc(dc uintptr, r rect, fill uintptr) {
	oldB, _, _ := selectObject.Call(dc, a.brush(fill))
	p, _, _ := createPen.Call(0, 1, fill)
	oldP, _, _ := selectObject.Call(dc, p)
	ellipse.Call(dc, uintptr(r.left), uintptr(r.top), uintptr(r.right), uintptr(r.bottom))
	selectObject.Call(dc, oldP)
	selectObject.Call(dc, oldB)
	deleteObject.Call(p)
}

func (a *appState) text(dc uintptr, r rect, text string, font int, color, flags uintptr) {
	if text == "" || r.right <= r.left || r.bottom <= r.top {
		return
	}
	old, _, _ := selectObject.Call(dc, a.fonts[font])
	setTextColor.Call(dc, color)
	setBkMode.Call(dc, 1)
	p := wide(text)
	drawText.Call(dc, uintptr(unsafe.Pointer(p)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), flags|dtNoPrefix)
	runtime.KeepAlive(p)
	selectObject.Call(dc, old)
}

func (a *appState) withPen(dc uintptr, color uintptr, width int32, draw func()) {
	p, _, _ := createPen.Call(0, uintptr(max(a.s(width), 1)), color)
	prev, _, _ := selectObject.Call(dc, p)
	draw()
	selectObject.Call(dc, prev)
	deleteObject.Call(p)
}

func line(dc uintptr, x0, y0, x1, y1 int32) {
	moveTo.Call(dc, uintptr(x0), uintptr(y0), 0)
	lineTo.Call(dc, uintptr(x1), uintptr(y1))
}

func (a *appState) icon(dc uintptr, kind string, x, y, size int32, color uintptr) {
	at := func(p point) point { return point{x + p.x*size/24, y + p.y*size/24} }
	segment := func(x0, y0, x1, y1 int32) {
		from, to := at(point{x0, y0}), at(point{x1, y1})
		line(dc, from.x, from.y, to.x, to.y)
	}
	curve := func(points ...point) {
		for i := range points {
			points[i] = at(points[i])
		}
		bezier.Call(dc, uintptr(unsafe.Pointer(&points[0])), uintptr(len(points)))
	}
	path := func(points ...point) {
		for i := 1; i < len(points); i++ {
			u, v := points[i-1], points[i]
			segment(u.x, u.y, v.x, v.y)
		}
	}
	stroke := int32(2)
	if kind == "brand" {
		stroke = 4
	}
	a.withPen(dc, color, stroke, func() {
		switch kind {
		case "brand":
			curve(point{1, 11}, point{5, 11}, point{9, 15}, point{10, 21})
			curve(point{10, 16}, point{11, 11}, point{15, 7}, point{22, 7})
			path(point{18, 3}, point{22, 7}, point{18, 11})
		case "link":
			curve(point{12, 9}, point{9, 6}, point{7, 8}, point{5, 10},
				point{3, 12}, point{0, 14}, point{3, 18}, point{5, 21}, point{8, 20}, point{10, 18})
			curve(point{12, 15}, point{15, 18}, point{17, 16}, point{19, 14},
				point{21, 12}, point{24, 10}, point{21, 6}, point{19, 3}, point{16, 4}, point{14, 6})
			segment(9, 15, 15, 9)
		case "settings":
			path(point{9, 2}, point{15, 2}, point{16, 5}, point{19, 5}, point{22, 10},
				point{20, 12}, point{22, 15}, point{19, 20}, point{16, 19}, point{15, 22},
				point{9, 22}, point{8, 19}, point{5, 20}, point{2, 15}, point{4, 12},
				point{2, 10}, point{5, 5}, point{8, 5}, point{9, 2})
			curve(point{16, 12}, point{16, 17}, point{8, 17}, point{8, 12},
				point{8, 7}, point{16, 7}, point{16, 12})
		case "copy":
			path(point{8, 5}, point{4, 5}, point{4, 22}, point{20, 22}, point{20, 5}, point{16, 5})
			path(point{8, 3}, point{16, 3}, point{16, 7}, point{8, 7}, point{8, 3})
		case "arrow":
			segment(2, 12, 21, 12)
			path(point{13, 4}, point{21, 12}, point{13, 20})
		case "info":
			curve(point{21, 12}, point{21, 24}, point{3, 24}, point{3, 12},
				point{3, 0}, point{21, 0}, point{21, 12})
			segment(12, 10, 12, 17)
			segment(12, 6, 12, 8)
		}
	})
}

func (a *appState) paint(dc uintptr) {
	if a.hwnd == 0 || dc == 0 {
		return
	}
	r, railWidth, left, right := a.dimensions()
	w := right - left
	a.fill(dc, r, surface)
	a.fill(dc, rect{0, 0, railWidth, r.bottom}, rail)
	if r.right >= a.s(940) {
		a.icon(dc, "brand", a.s(29), a.s(26), a.s(39), accent)
		a.text(dc, rect{a.s(77), a.s(23), railWidth - a.s(7), a.s(68)}, "LinkSwitch", fontBrand, ink, dtSingleLine|dtVCenter)
	} else {
		a.icon(dc, "brand", a.s(18), a.s(27), a.s(31), accent)
		a.text(dc, rect{a.s(55), a.s(21), railWidth - a.s(7), a.s(64)}, "LinkSwitch", fontRow, ink, dtSingleLine|dtVCenter)
	}

	titleFont := fontTitle
	defaultWidth := a.s(201)
	if r.right < a.s(940) {
		titleFont, defaultWidth = fontCompactTitle, a.s(174)
	}
	title := "どのブラウザーで開きますか？"
	if a.page == pageSettings {
		title = "ブラウザー設定"
	}
	a.text(dc, rect{left, a.s(27), right - defaultWidth - a.s(12), a.s(89)}, title, titleFont, ink, dtWordBreak)

	if a.page == pageOpen {
		a.text(dc, a.rect(left, a.s(77), w, a.s(27)), "開くリンク", fontSection, ink, dtSingleLine|dtVCenter)
		bg := a.rect(left, a.s(112), w, a.s(65))
		border := uintptr(linkBorder)
		if focused, _, _ := getFocus.Call(); focused == a.controls[idURL] {
			border = accent
		}
		a.roundShape(dc, bg, a.s(15), linkBackground, border)
		a.icon(dc, "link", left+a.s(19), a.s(130), a.s(27), accent)
		if len(a.cfg.Browsers) == 0 {
			box := a.rect(left, a.s(204), w, a.s(104))
			// The empty list has no items; its overlay is not a child window.
			a.roundShape(dc, box, a.s(14), white, edge)
			a.text(dc, a.rect(left+a.s(19), a.s(217), w-a.s(38), a.s(25)), "まだブラウザーが登録されていません", fontSection, ink, dtSingleLine|dtVCenter|dtEllipsis)
			a.text(dc, a.rect(left+a.s(19), a.s(247), w-a.s(38), a.s(42)), "「ブラウザー設定」で検出するか、実行ファイルを指定してください。", fontBody, muted, dtWordBreak)
		}
	} else {
		a.paintSettings(dc, left, right, r.bottom < a.s(444))
	}
	y := a.footerTop
	a.fill(dc, rect{railWidth, y, r.right, y + 1}, divider)
	a.icon(dc, "info", left, y+a.s(21), a.s(20), muted)
	status, color := a.status, uintptr(muted)
	if status == "" {
		if a.page == pageOpen {
			status = "ブラウザーを選び、矢印ボタンまたは Enter でリンクを開きます。"
		} else if a.dirty {
			status = "変更はまだ保存されていません。"
		} else {
			status = "ここで登録した順に、リンクを開く画面に表示されます。"
		}
	}
	if a.statusWarn {
		color = warning
	}
	buttonWidth := a.s(120)
	if a.page == pageSettings {
		buttonWidth = a.s(164)
	}
	a.text(dc, rect{left + a.s(28), y + a.s(12), right - buttonWidth - a.s(10), r.bottom - a.s(10)}, status, fontSmall, color, dtWordBreak)
}

func (a *appState) paintSettings(dc uintptr, left, right int32, compact bool) {
	fx, fw := a.formLeft, a.formWidth
	g := settingsGeometry(compact)
	subtop, subheight := int32(77), int32(26)
	if compact {
		subtop, subheight = 70, 21
	}
	a.text(dc, a.rect(left, a.s(subtop), right-left, a.s(subheight)), "ブラウザーやプロファイルを追加して、リンクの開き先を選べます。", fontBody, muted, dtSingleLine|dtVCenter|dtEllipsis)
	a.text(dc, a.rect(fx, a.s(g.nameLabel), fw, a.s(23)), "表示名", fontSection, ink, dtSingleLine|dtVCenter)
	a.text(dc, a.rect(fx, a.s(g.exeLabel), fw, a.s(23)), "実行ファイル (.exe)", fontSection, ink, dtSingleLine|dtVCenter)
	a.text(dc, a.rect(fx, a.s(g.argsLabel), fw, a.s(22)), "起動引数（任意）", fontSection, ink, dtSingleLine|dtVCenter)
	a.text(dc, a.rect(fx, a.s(g.argsHelp), fw, a.s(23)), "1行に1引数。{url} はリンクに置換。", fontSmall, muted, dtWordBreak)
	if len(a.cfg.Browsers) == 0 {
		lw := fx - left - a.s(23)
		a.roundShape(dc, a.rect(left, a.s(g.list), lw, a.s(108)), a.s(12), white, edge)
		a.text(dc, a.rect(left+a.s(12), a.s(g.list+12), lw-a.s(24), a.s(83)), "まず「ブラウザーを検出」を押すか、「新規」で手動追加します。", fontBody, muted, dtWordBreak)
	}
}

func (a *appState) arrowRect(r rect) rect {
	w, h := a.s(46), a.s(37)
	top := r.top + (r.bottom-r.top-h)/2 - a.s(3)
	return rect{r.right - w - a.s(21), top, r.right - a.s(21), top + h}
}

func (a *appState) drawControl(di *drawItem) {
	if di.controlType == 2 {
		a.drawBrowser(di)
	} else if di.controlType == 4 {
		a.drawButton(di)
	}
}

func (a *appState) drawBrowser(di *drawItem) {
	r, dc := di.rc, di.hdc
	a.fill(dc, r, surface)
	i := int(di.item)
	if i >= len(a.cfg.Browsers) {
		return
	}
	b := a.cfg.Browsers[i]
	bg, stroke := uintptr(white), uintptr(edge)
	if di.state&odsSelected != 0 {
		bg, stroke = rowSelected, linkBorder
	}
	if di.state&odsFocus != 0 {
		stroke = accent
	}
	settings := int(di.id) == idProfiles
	card := rect{r.left + a.s(2), r.top + a.s(2), r.right - a.s(1), r.bottom - a.s(7)}
	a.roundShape(dc, card, a.s(15), bg, stroke)
	monogram, badge := badgeFor(b)
	if settings {
		d := a.s(26)
		bx, by := card.left+a.s(10), card.top+(card.bottom-card.top-d)/2
		a.disc(dc, a.rect(bx, by, d, d), badge)
		a.text(dc, a.rect(bx, by-a.s(1), d, d), monogram, fontBody, white, dtCenter|dtVCenter|dtSingleLine)
		a.text(dc, rect{bx + d + a.s(9), card.top, card.right - a.s(7), card.bottom}, b.Name, fontBody, ink, dtVCenter|dtSingleLine|dtEllipsis)
		return
	}
	d := a.s(50)
	bx, by := card.left+a.s(19), card.top+(card.bottom-card.top-d)/2
	a.disc(dc, a.rect(bx, by, d, d), badge)
	a.text(dc, a.rect(bx, by-a.s(1), d, d), monogram, fontBadge, white, dtCenter|dtVCenter|dtSingleLine)
	arrow := a.arrowRect(r)
	x := bx + d + a.s(20)
	a.text(dc, rect{x, card.top + a.s(12), arrow.left - a.s(9), card.top + a.s(40)}, b.Name, fontRow, ink, dtSingleLine|dtVCenter|dtEllipsis)
	a.text(dc, rect{x, card.top + a.s(41), arrow.left - a.s(9), card.top + a.s(66)}, profileLabel(b), fontBody, muted, dtSingleLine|dtVCenter|dtEllipsis)
	a.roundShape(dc, arrow, a.s(13), accent, accent)
	a.icon(dc, "arrow", arrow.left+(arrow.right-arrow.left-a.s(21))/2, arrow.top+(arrow.bottom-arrow.top-a.s(21))/2, a.s(21), white)
}

func (a *appState) drawButton(di *drawItem) {
	id, r, dc := int(di.id), di.rc, di.hdc
	var icon string
	fill, border, fg, behind := uintptr(white), uintptr(edge), uintptr(ink), uintptr(surface)
	prefix, centered := a.s(0), true
	switch id {
	case idOpen, idSettings:
		behind, fill, border, centered = rail, rail, rail, false
		active := (id == idOpen && a.page == pageOpen) || (id == idSettings && a.page == pageSettings)
		if active {
			fill, border = selectedRail, selectedRail
		}
		if id == idOpen {
			icon = "link"
		} else {
			icon = "settings"
		}
	case idDefaults:
		icon = "settings"
	case idCopy:
		icon, behind = "copy", linkBackground
	case idDiscover, idSave:
		fill, border, fg = accent, accent, white
	}
	if di.state&odsDisabled != 0 {
		fill, border, fg = disabledFill, divider, disabledInk
	} else if di.state&odsSelected != 0 {
		if fg == white {
			fill, border = accentDark, accentDark
		} else {
			fill, border = rowSelected, linkBorder
		}
	}
	if di.state&odsFocus != 0 && di.state&odsNoFocusRect == 0 && di.state&odsDisabled == 0 {
		border = accent
		if fill == accent {
			border = ink
		}
	}
	a.fill(dc, r, behind)
	a.roundShape(dc, r, a.s(13), fill, border)
	if (id == idOpen && a.page == pageOpen) || (id == idSettings && a.page == pageSettings) {
		a.roundShape(dc, a.rect(r.left, r.top+a.s(8), a.s(5), r.bottom-r.top-a.s(16)), a.s(10), accent, accent)
	}
	if icon != "" {
		x := r.left + a.s(18)
		if (id == idDefaults && r.right-r.left < a.s(180)) || (!centered && r.right-r.left < a.s(170)) {
			x = r.left + a.s(11)
		}
		if id == idCopy {
			x = r.left + a.s(18)
		}
		a.icon(dc, icon, x, r.top+(r.bottom-r.top-a.s(20))/2, a.s(20), fg)
		prefix = x - r.left + a.s(28)
	}
	box := rect{r.left + prefix + a.s(3), r.top, r.right - a.s(3), r.bottom}
	alignment := uintptr(dtLeft)
	if centered && icon == "" {
		alignment = dtCenter
	}
	a.text(dc, box, readText(di.hwnd), fontButton, fg, alignment|dtVCenter|dtSingleLine|dtEllipsis)
}

func profileLabel(b core.Browser) string {
	for i, arg := range b.Args {
		if strings.HasPrefix(arg, "--profile-directory=") {
			if profile := strings.TrimPrefix(arg, "--profile-directory="); profile != "" {
				return "プロファイル: " + profile
			}
		}
		if (arg == "-P" || arg == "--profile-directory") && i+1 < len(b.Args) {
			return "プロファイル: " + b.Args[i+1]
		}
	}
	if len(b.Args) == 0 {
		return "プロファイル: 既定"
	}
	return "プロファイル: カスタム"
}

func badgeFor(b core.Browser) (string, uintptr) {
	switch strings.ToLower(filepath.Base(b.Executable)) {
	case "msedge.exe", "msedge":
		return "E", accent
	case "firefox.exe", "firefox":
		return "F", orange
	case "chrome.exe", "chrome":
		return "C", red
	case "brave.exe", "brave":
		return "B", orange
	case "vivaldi.exe", "vivaldi":
		return "V", red
	case "opera.exe", "opera":
		return "O", red
	case "chromium.exe", "chromium":
		return "C", teal
	default:
		first, _ := utf8.DecodeRuneInString(b.Name)
		if first == utf8.RuneError || unicode.IsSpace(first) {
			return "?", purple
		}
		return string(unicode.ToUpper(first)), purple
	}
}
