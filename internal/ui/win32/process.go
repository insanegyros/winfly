package win32

import (
	"fmt"
	"unsafe"
)

// ModuleHandle returns the handle of the running executable.
func ModuleHandle() uintptr {
	handle, _, _ := procGetModuleHandleW.Call(0)
	return handle
}

// InitCommonControls loads the standard controls and the tab control. Together
// with the application manifest this gives the controls the modern Windows look.
func InitCommonControls() error {
	options := initCommonControlsEx{
		Size: uint32(unsafe.Sizeof(initCommonControlsEx{})),
		ICC:  ICC_STANDARD_CLASSES | ICC_TAB_CLASSES,
	}
	ok, _, err := procInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&options)))
	if ok == 0 {
		return fmt.Errorf("inicializace společných ovládacích prvků InitCommonControlsEx selhala: %w", callError(err))
	}
	return nil
}

// SystemDPI returns the dots per inch of the primary display: 96 at 100 %
// scaling, 120 at 125 %, 144 at 150 % and so on.
func SystemDPI() int32 {
	if procGetDpiForSystem.Find() != nil {
		return defaultDPI // Windows older than 10 version 1607.
	}
	dpi, _, _ := procGetDpiForSystem.Call()
	if dpi == 0 {
		return defaultDPI
	}
	return int32(dpi)
}

// Font is a handle to a GDI font.
type Font uintptr

// CreateFont creates a font of the given face name and size in points,
// rendered for the given DPI.
func CreateFont(faceName string, pointSize, dpi int32) Font {
	// A negative height asks for the character height (without the internal
	// leading), which is how point sizes are defined.
	height := -(pointSize*dpi + 36) / 72
	font, _, _ := procCreateFontW.Call(
		uintptr(height),
		0, 0, 0,
		fwNormal,
		0, 0, 0,
		defaultCharset,
		0, 0,
		clearTypeQual,
		0,
		uintptr(unsafe.Pointer(utf16Ptr(faceName))),
	)
	return Font(font)
}

// DeleteFont frees a font created with CreateFont.
func DeleteFont(font Font) {
	if font != 0 {
		procDeleteObject.Call(uintptr(font))
	}
}
