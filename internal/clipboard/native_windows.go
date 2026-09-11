//go:build windows

package clipboard

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"golang.org/x/sys/windows"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"
)

var (
	user32         = windows.NewLazySystemDLL("user32.dll")
	kernel32       = windows.NewLazySystemDLL("kernel32.dll")
	openClipboard  = user32.NewProc("OpenClipboard")
	closeClipboard = user32.NewProc("CloseClipboard")
	isAvailable    = user32.NewProc("IsClipboardFormatAvailable")
	getData        = user32.NewProc("GetClipboardData")
	setData        = user32.NewProc("SetClipboardData")
	emptyClipboard = user32.NewProc("EmptyClipboard")
	sequenceNumber = user32.NewProc("GetClipboardSequenceNumber")
	registerFormat = user32.NewProc("RegisterClipboardFormatW")
	dragQueryFile  = windows.NewLazySystemDLL("shell32.dll").NewProc("DragQueryFileW")
	createWindow   = user32.NewProc("CreateWindowExW")
	destroyWindow  = user32.NewProc("DestroyWindow")
	globalAlloc    = kernel32.NewProc("GlobalAlloc")
	globalFree     = kernel32.NewProc("GlobalFree")
	globalLock     = kernel32.NewProc("GlobalLock")
	globalUnlock   = kernel32.NewProc("GlobalUnlock")
	globalSize     = kernel32.NewProc("GlobalSize")
)

func open(owner uintptr) error {
	for i := 0; i < 5; i++ {
		if ok, _, _ := openClipboard.Call(owner); ok != 0 {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("剪貼簿正被其他程式使用")
}

func nativeRevision() int64 { n, _, _ := sequenceNumber.Call(); return int64(n) }
func pngFormat() uintptr {
	name, _ := windows.UTF16PtrFromString("PNG")
	id, _, _ := registerFormat.Call(uintptr(unsafe.Pointer(name)))
	return id
}
func available(format uintptr) bool {
	n, _, _ := isAvailable.Call(format)
	return format != 0 && n != 0
}
func readGlobal(format uintptr, limit uintptr) ([]byte, error) {
	h, _, _ := getData.Call(format)
	size, _, _ := globalSize.Call(h)
	if h == 0 || size == 0 || size > limit {
		return nil, fmt.Errorf("剪貼簿資料大小無效")
	}
	ptr, _, _ := globalLock.Call(h)
	if ptr == 0 {
		return nil, fmt.Errorf("無法鎖定剪貼簿")
	}
	defer globalUnlock.Call(h)
	return append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(ptr)), int(size))...), nil
}
func readContent() (content, error)    { return readNativeContent(false) }
func readLegacyText() (content, error) { return readNativeContent(true) }
func readNativeContent(textOnly bool) (content, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := open(0); err != nil {
		return content{}, err
	}
	defer closeClipboard.Call()
	if textOnly && (available(15) || available(pngFormat()) || available(8) || available(17) || available(2)) {
		return content{Kind: "unsupported"}, nil
	}
	if available(15) {
		h, _, _ := getData.Call(15)
		count, _, _ := dragQueryFile.Call(h, 0xffffffff, 0, 0)
		if count == 0 || count > MaxFiles {
			return content{}, fmt.Errorf("剪貼簿檔案數量超過限制")
		}
		paths := make([]string, 0, count)
		for i := uintptr(0); i < count; i++ {
			n, _, _ := dragQueryFile.Call(h, i, 0, 0)
			if n == 0 || n > 32767 {
				return content{}, fmt.Errorf("剪貼簿檔案路徑無效")
			}
			name := make([]uint16, n+1)
			dragQueryFile.Call(h, i, uintptr(unsafe.Pointer(&name[0])), n+1)
			paths = append(paths, windows.UTF16ToString(name))
		}
		return content{Kind: "files", Paths: paths}, nil
	}
	if format := pngFormat(); available(format) {
		data, err := readGlobal(format, MaxImageBytes)
		return content{Kind: "image", Data: data}, err
	}
	// Windows 可將 CF_BITMAP／CF_DIBV5 自動轉為 CF_DIB。
	if available(8) || available(17) || available(2) {
		data, err := readGlobal(8, 4*MaxPixels+1024*1024)
		if err != nil {
			return content{}, err
		}
		img, err := decodeDIB(data)
		if err != nil {
			return content{}, err
		}
		var output bytes.Buffer
		if err = png.Encode(&output, img); err != nil {
			return content{}, err
		}
		return content{Kind: "image", Data: output.Bytes()}, nil
	}
	if !available(13) {
		return content{}, nil
	}
	data, err := readGlobal(13, 2*(MaxTextBytes+1))
	if err != nil {
		return content{}, err
	}
	words := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		v := binary.LittleEndian.Uint16(data[i:])
		if v == 0 {
			return content{Kind: "text", Data: []byte(strings.ReplaceAll(windows.UTF16ToString(words), "\r\n", "\n"))}, nil
		}
		words = append(words, v)
	}
	return content{}, fmt.Errorf("剪貼簿文字未終止")
}
func setGlobal(format uintptr, data []byte) error {
	h, _, _ := globalAlloc.Call(0x0002, uintptr(len(data)))
	if h == 0 {
		return fmt.Errorf("剪貼簿記憶體配置失敗")
	}
	owned := true
	defer func() {
		if owned {
			globalFree.Call(h)
		}
	}()
	ptr, _, _ := globalLock.Call(h)
	if ptr == 0 {
		return fmt.Errorf("無法鎖定剪貼簿記憶體")
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(ptr)), len(data)), data)
	globalUnlock.Call(h)
	if ok, _, _ := setData.Call(format, h); ok == 0 {
		return fmt.Errorf("剪貼簿寫入失敗")
	}
	owned = false
	return nil
}
func writeContent(value content) error {
	var data, dib []byte
	format := uintptr(13)
	switch value.Kind {
	case "text":
		text := strings.ReplaceAll(string(value.Data), "\r\n", "\n")
		text = strings.ReplaceAll(text, "\r", "\n")
		text = strings.ReplaceAll(text, "\n", "\r\n")
		words, err := windows.UTF16FromString(text)
		if err != nil {
			return err
		}
		data = make([]byte, len(words)*2)
		for i, v := range words {
			binary.LittleEndian.PutUint16(data[i*2:], v)
		}
	case "image":
		if err := validImage(value.Data); err != nil {
			return err
		}
		img, err := png.Decode(bytes.NewReader(value.Data))
		if err != nil {
			return err
		}
		dib = encodeDIB(img)
		data = value.Data
		format = pngFormat()
	case "files":
		if len(value.Paths) == 0 || len(value.Paths) > MaxFiles {
			return fmt.Errorf("剪貼簿檔案項目數量無效")
		}
		format = 15
		data = make([]byte, 20)
		binary.LittleEndian.PutUint32(data, 20)
		binary.LittleEndian.PutUint32(data[16:], 1)
		for _, path := range value.Paths {
			if !filepath.IsAbs(path) {
				return fmt.Errorf("剪貼簿檔案必須使用完整路徑")
			}
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("剪貼簿項目不存在：%w", err)
			}
			words, err := windows.UTF16FromString(path)
			if err != nil {
				return err
			}
			for _, v := range words {
				data = append(data, byte(v), byte(v>>8))
			}
		}
		data = append(data, 0, 0)
	default:
		return fmt.Errorf("剪貼簿格式不支援")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// SetClipboardData 使用本程序擁有的 message-only window，不借用桌面 HWND。
	class, _ := windows.UTF16PtrFromString("STATIC")
	owner, _, err := createWindow.Call(0, uintptr(unsafe.Pointer(class)), 0, 0, 0, 0, 0, 0, ^uintptr(2), 0, 0, 0)
	if owner == 0 {
		return fmt.Errorf("無法建立剪貼簿擁有者：%w", err)
	}
	defer destroyWindow.Call(owner)
	if err := open(owner); err != nil {
		return err
	}
	defer closeClipboard.Call()
	if ok, _, _ := emptyClipboard.Call(); ok == 0 {
		return fmt.Errorf("無法清空剪貼簿")
	}
	if len(dib) > 0 {
		if err := setGlobal(8, dib); err != nil {
			return err
		}
	}
	if err := setGlobal(format, data); err != nil {
		return err
	}
	if value.Kind == "files" {
		// 明確告知檔案總管這是複製，不是剪下或移動。
		name, _ := windows.UTF16PtrFromString("Preferred DropEffect")
		effect, _, _ := registerFormat.Call(uintptr(unsafe.Pointer(name)))
		if effect == 0 {
			return fmt.Errorf("無法註冊檔案複製格式")
		}
		if err := setGlobal(effect, []byte{1, 0, 0, 0}); err != nil {
			return err
		}
		handle, _, _ := getData.Call(15)
		count, _, _ := dragQueryFile.Call(handle, 0xffffffff, 0, 0)
		if handle == 0 || int(count) != len(value.Paths) {
			return fmt.Errorf("Windows 未保留完整的剪貼簿檔案清單")
		}
	}
	return nil
}
