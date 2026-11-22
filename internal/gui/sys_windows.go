package gui

import (
	"syscall"
	wutf16 "unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows/registry"
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
