package gui

import (
	"image/color"
	"os/exec"
	"path/filepath"
	"syscall"
	wutf16 "unicode/utf16"
	"unsafe"

	"gioui.org/app"
	"gioui.org/io/event"
	"golang.org/x/sys/windows/registry"

	"soverter/internal/gui/theme"
)

// systemLight reports whether Windows apps are set to the light theme.
func systemLight() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AppsUseLightTheme")
	return err == nil && v == 1
}

type openFileName struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	flagsEx           uint32
}

// PickVideoFile shows the standard Windows "open file" dialog; ok is false if the user cancelled.
func PickVideoFile(title string) (path string, ok bool) {
	filter := utf16("Видео\x00*.mp4;*.mkv;*.mov;*.avi;*.webm;*.m4v;*.ts;*.wmv;*.flv\x00Все файлы\x00*.*\x00")
	buf := make([]uint16, 4096)
	t, _ := syscall.UTF16PtrFromString(title)
	ofn := openFileName{
		lpstrFilter: &filter[0], nFilterIndex: 1,
		lpstrFile: &buf[0], nMaxFile: uint32(len(buf)), lpstrTitle: t,
		flags: 0x1000 | 0x800 | 0x80000 | 0x8, // FILEMUSTEXIST | PATHMUSTEXIST | EXPLORER | NOCHANGEDIR
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	r, _, _ := syscall.NewLazyDLL("comdlg32.dll").NewProc("GetOpenFileNameW").Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

// utf16 converts s to UTF-16 keeping embedded NULs (needed for double-NUL filter strings).
func utf16(s string) []uint16 {
	return append(wutf16.Encode([]rune(s)), 0)
}

// ShowInFolder opens Explorer with the file selected. The command line is built by hand: Go would quote the
// whole "/select,path" argument, which Explorer does not understand (it then just opens its default folder).
func ShowInFolder(path string) {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + path + `"`}
	_ = cmd.Start()
}

// DWM window attributes (Windows 11; older systems ignore the colour ones).
const (
	dwmUseImmersiveDarkMode = 20
	dwmBorderColor          = 34
	dwmCaptionColor         = 35
	dwmTextColor            = 36
)

func colorRef(c color.NRGBA) uint32 { return uint32(c.R) | uint32(c.G)<<8 | uint32(c.B)<<16 } // 0x00BBGGRR

// styleChrome tints the native title bar (with the minimise, maximise and close buttons) to match the app: the
// caption takes the sidebar colour, a shade away from the window background so the bar reads as a frame rather
// than melting into the content, and the border and caption text follow the palette. The window handle arrives
// as a Win32ViewEvent right after the window is created (see windowHandle).
func styleChrome(hwnd uintptr, p theme.Palette) {
	dark := uint32(0)
	if p.Dark {
		dark = 1
	}
	set := func(attr, val uint32) {
		syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute").Call(hwnd, uintptr(attr), uintptr(unsafe.Pointer(&val)), 4)
	}
	set(dwmUseImmersiveDarkMode, dark) // light glyphs on a dark bar and vice versa
	set(dwmCaptionColor, colorRef(p.Sidebar))
	set(dwmBorderColor, colorRef(p.Raised))
	set(dwmTextColor, colorRef(p.TextDim))
}

// windowHandle returns the HWND carried by a Win32ViewEvent, or 0 for any other event.
func windowHandle(ev event.Event) uintptr {
	if v, ok := ev.(app.Win32ViewEvent); ok && v.Valid() {
		return v.HWND
	}
	return 0
}
