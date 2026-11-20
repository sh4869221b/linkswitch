//go:build windows && (amd64 || arm64)

package main

import (
	"runtime"
	"testing"
	"unsafe"
)

func TestArguments(t *testing.T) {
	const url = "https://example.com/search?q=one%20two&lang=ja#top"
	for _, tc := range []struct {
		name string
		args []string
		want string
		ok   bool
	}{
		{"manual", nil, "", true},
		{"direct", []string{url}, url, true},
		{"registered", []string{"--url", url}, url, true},
		{"incomplete", []string{"--url"}, "", false},
		{"flag injection", []string{url, "--incognito"}, "", false},
		{"unexpected trailing", []string{"--url", url, "https://example.org"}, "", false},
		{"wrong switch", []string{"--other", url}, "", false},
		{"file", []string{"--url", "file:///C:/Windows/win.ini"}, "", false},
		{"empty", []string{"--url", ""}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := arguments(tc.args)
			if ok != tc.ok || (ok && got != tc.want) {
				t.Errorf("arguments(%q) = %q, %v; want %q, %v", tc.args, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestWin32ABI(t *testing.T) {
	for _, tc := range []struct {
		structure string
		got, want uintptr
	}{
		{"WNDCLASSEXW", unsafe.Sizeof(wndclass{}), 80},
		{"MSG", unsafe.Sizeof(msg{}), 48},
		{"PAINTSTRUCT", unsafe.Sizeof(paintStruct{}), 72},
		{"DRAWITEMSTRUCT", unsafe.Sizeof(drawItem{}), 64},
		{"MINMAXINFO", unsafe.Sizeof(minMaxInfo{}), 40},
		{"OPENFILENAMEW", unsafe.Sizeof(openFileName{}), 152},
	} {
		if tc.got != tc.want {
			t.Errorf("%s size = %d; want %d", tc.structure, tc.got, tc.want)
		}
	}
	r := rect{1, 2, 3, 4}
	var pinned runtime.Pinner
	pinned.Pin(&r)
	defer pinned.Unpin()
	address := uintptr(unsafe.Pointer(&r))
	if got := readNative[rect](address); got != r {
		t.Errorf("Win32 memory read = %+v; want %+v", got, r)
	}
	want := rect{17, 21, 45, 183}
	writeNative(address, &want)
	if r != want {
		t.Errorf("Win32 memory write = %+v; want %+v", r, want)
	}
}
