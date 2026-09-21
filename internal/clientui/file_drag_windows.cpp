//go:build windows && cgo

#ifndef NOMINMAX
#define NOMINMAX
#endif
#ifndef UNICODE
#define UNICODE
#endif
#include <windows.h>
#include <windowsx.h>
#include <ole2.h>
#include <shlobj.h>
#include <commctrl.h>
#include <atomic>
#include <algorithm>
#include <cstring>
#include <cstdlib>
#include <new>
#include <string>
#include <utility>
#include <vector>

// CF_HDROP contains only already downloaded local files. No clipboard API is
// called; each native drag gets its own immutable copy of this data object.
class FileDragData final : public IDataObject {
    std::atomic<ULONG> refs{1};
    std::vector<wchar_t> paths;
    CLIPFORMAT preferred = (CLIPFORMAT)RegisterClipboardFormatW(L"Preferred DropEffect");
public:
    explicit FileDragData(const std::vector<std::wstring>& files) {
        for (const auto& file : files) {
            paths.insert(paths.end(), file.begin(), file.end());
            paths.push_back(0);
        }
        paths.push_back(0);
    }
    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID id, void **out) override {
        if (!out) return E_POINTER;
        *out = nullptr;
        if (id != IID_IUnknown && id != IID_IDataObject) return E_NOINTERFACE;
        *out = static_cast<IDataObject *>(this); AddRef(); return S_OK;
    }
    ULONG STDMETHODCALLTYPE AddRef() override { return ++refs; }
    ULONG STDMETHODCALLTYPE Release() override { ULONG n = --refs; if (!n) delete this; return n; }
    HRESULT STDMETHODCALLTYPE QueryGetData(FORMATETC *format) override {
        if (!format) return E_POINTER;
        if (format->dwAspect != DVASPECT_CONTENT) return DV_E_DVASPECT;
        if (format->lindex != -1) return DV_E_LINDEX;
        if (!(format->tymed & TYMED_HGLOBAL)) return DV_E_TYMED;
        return format->cfFormat == CF_HDROP || format->cfFormat == preferred ? S_OK : DV_E_FORMATETC;
    }
    HRESULT STDMETHODCALLTYPE GetData(FORMATETC *format, STGMEDIUM *medium) override {
        if (!medium) return E_POINTER;
        ZeroMemory(medium, sizeof(*medium));
        HRESULT hr = QueryGetData(format);
        if (FAILED(hr)) return hr;
        size_t bytes = format->cfFormat == preferred ? sizeof(DWORD) : sizeof(DROPFILES)+paths.size()*sizeof(wchar_t);
        HGLOBAL memory = GlobalAlloc(GMEM_MOVEABLE | GMEM_ZEROINIT, bytes);
        if (!memory) return E_OUTOFMEMORY;
        void *buffer = GlobalLock(memory);
        if (!buffer) { GlobalFree(memory); return E_OUTOFMEMORY; }
        if (format->cfFormat == preferred) {
            *static_cast<DWORD *>(buffer) = DROPEFFECT_COPY;
        } else {
            auto drop = static_cast<DROPFILES *>(buffer);
            drop->pFiles = sizeof(DROPFILES);
            drop->fWide = TRUE;
            memcpy(static_cast<char *>(buffer)+sizeof(DROPFILES), paths.data(), paths.size()*sizeof(wchar_t));
        }
        GlobalUnlock(memory);
        medium->tymed = TYMED_HGLOBAL;
        medium->hGlobal = memory;
        return S_OK;
    }
    HRESULT STDMETHODCALLTYPE GetDataHere(FORMATETC *, STGMEDIUM *) override { return DATA_E_FORMATETC; }
    HRESULT STDMETHODCALLTYPE GetCanonicalFormatEtc(FORMATETC *, FORMATETC *out) override {
        if (!out) return E_POINTER;
        ZeroMemory(out, sizeof(*out)); return E_NOTIMPL;
    }
    HRESULT STDMETHODCALLTYPE SetData(FORMATETC *, STGMEDIUM *medium, BOOL release) override {
        // A shell target may report its completed effect. This source never
        // removes or moves files, even if a target requests a move.
        if (release && medium) ReleaseStgMedium(medium);
        return S_OK;
    }
    HRESULT STDMETHODCALLTYPE EnumFormatEtc(DWORD direction, IEnumFORMATETC **out) override {
        if (!out) return E_POINTER;
        *out = nullptr;
        if (direction != DATADIR_GET) return E_NOTIMPL;
        FORMATETC formats[] = {{CF_HDROP, nullptr, DVASPECT_CONTENT, -1, TYMED_HGLOBAL},
                              {preferred, nullptr, DVASPECT_CONTENT, -1, TYMED_HGLOBAL}};
        return SHCreateStdEnumFmtEtc(2, formats, out);
    }
    HRESULT STDMETHODCALLTYPE DAdvise(FORMATETC *, DWORD, IAdviseSink *, DWORD *) override { return OLE_E_ADVISENOTSUPPORTED; }
    HRESULT STDMETHODCALLTYPE DUnadvise(DWORD) override { return OLE_E_ADVISENOTSUPPORTED; }
    HRESULT STDMETHODCALLTYPE EnumDAdvise(IEnumSTATDATA **) override { return OLE_E_ADVISENOTSUPPORTED; }
};

class FileDragSource final : public IDropSource {
    std::atomic<ULONG> refs{1};
    HWND window;
public:
    explicit FileDragSource(HWND source) : window(source) {}
    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID id, void **out) override {
        if (!out) return E_POINTER;
        *out = nullptr;
        if (id != IID_IUnknown && id != IID_IDropSource) return E_NOINTERFACE;
        *out = static_cast<IDropSource *>(this); AddRef(); return S_OK;
    }
    ULONG STDMETHODCALLTYPE AddRef() override { return ++refs; }
    ULONG STDMETHODCALLTYPE Release() override { ULONG n = --refs; if (!n) delete this; return n; }
    HRESULT STDMETHODCALLTYPE QueryContinueDrag(BOOL escape, DWORD keys) override {
        if (escape || !IsWindow(window) || (keys & MK_RBUTTON)) return DRAGDROP_S_CANCEL;
        return keys & MK_LBUTTON ? S_OK : DRAGDROP_S_DROP;
    }
    HRESULT STDMETHODCALLTYPE GiveFeedback(DWORD) override { return DRAGDROP_S_USEDEFAULTCURSORS; }
};

struct FileDragView {
    ULONG refs = 1;
    HWND parent = nullptr, window = nullptr;
    DWORD thread = GetCurrentThreadId();
    std::vector<std::wstring> files;
    POINT origin{};
    bool pressed = false, dragging = false, invalid = false, ole = false;
    void retain() { ++refs; }
    void release() { if (!--refs) delete this; }
    ~FileDragView() { if (ole) OleUninitialize(); }
};

static int dragScale(HWND window, int value) {
    typedef UINT (WINAPI *GetDpi)(HWND);
    static auto dpi = reinterpret_cast<GetDpi>(GetProcAddress(GetModuleHandleW(L"user32.dll"), "GetDpiForWindow"));
    UINT density = dpi ? dpi(window) : 96;
    return MulDiv(value, density ? density : 96, 96);
}

static void positionFileDrag(FileDragView *view) {
    if (!view->window || !view->parent) return;
    RECT bounds{};
    if (!GetClientRect(view->parent, &bounds)) return;
    int height = dragScale(view->parent, 72);
    SetWindowPos(view->window, HWND_TOP, 0, std::max(0L, bounds.bottom-height), bounds.right, height, SWP_NOACTIVATE);
}

static void beginFileDrag(FileDragView *view) {
    if (view->invalid || view->dragging || view->files.empty()) return;
    FileDragData *data = nullptr;
    FileDragSource *source = nullptr;
    try {
        // Windows can run dispatched cleanup while DoDragDrop pumps messages.
        data = new FileDragData(view->files);
        source = new FileDragSource(view->window);
    } catch (...) {
        if (data) data->Release();
        return;
    }
    view->retain();
    view->dragging = true;
    view->pressed = false;
    if (GetCapture() == view->window) ReleaseCapture();
    DWORD effect = DROPEFFECT_NONE;
    DoDragDrop(data, source, DROPEFFECT_COPY, &effect);
    source->Release();
    data->Release();
    view->dragging = false;
    view->release();
}

static LRESULT CALLBACK fileDragProcedure(HWND window, UINT message, WPARAM wParam, LPARAM lParam) {
    auto view = reinterpret_cast<FileDragView *>(GetWindowLongPtrW(window, GWLP_USERDATA));
    if (message == WM_NCCREATE) {
        view = static_cast<FileDragView *>(reinterpret_cast<CREATESTRUCTW *>(lParam)->lpCreateParams);
        SetWindowLongPtrW(window, GWLP_USERDATA, reinterpret_cast<LONG_PTR>(view));
        view->window = window;
    }
    if (!view) return DefWindowProcW(window, message, wParam, lParam);
    switch (message) {
    case WM_PAINT: {
        PAINTSTRUCT paint{};
        HDC dc = BeginPaint(window, &paint);
        RECT rect{}; GetClientRect(window, &rect);
        FillRect(dc, &rect, GetSysColorBrush(COLOR_BTNFACE));
        RECT border = rect; border.bottom = border.top+1;
        FillRect(dc, &border, GetSysColorBrush(COLOR_3DSHADOW));
        SetBkMode(dc, TRANSPARENT);
        SetTextColor(dc, GetSysColor(view->files.empty() ? COLOR_GRAYTEXT : COLOR_BTNTEXT));
        HFONT font = CreateFontW(-dragScale(window, 14), 0, 0, 0, FW_MEDIUM, FALSE, FALSE, FALSE,
            DEFAULT_CHARSET, OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS, CLEARTYPE_QUALITY, DEFAULT_PITCH, L"Segoe UI");
        HGDIOBJ previous = font ? SelectObject(dc, font) : nullptr;
        rect.left += dragScale(window, 12); rect.right -= dragScale(window, 12);
        rect.top = dragScale(window, 10); rect.bottom = dragScale(window, 37);
        std::wstring title = view->files.empty() ? L"先下載檔案，再從這裡拖到 Explorer"
            : L"拖曳已備妥的 " + std::to_wstring(view->files.size()) + L" 個檔案到 Explorer";
        DrawTextW(dc, title.c_str(), -1, &rect, DT_CENTER | DT_SINGLELINE | DT_VCENTER | DT_END_ELLIPSIS);
        SetTextColor(dc, GetSysColor(COLOR_GRAYTEXT));
        rect.top = dragScale(window, 38); rect.bottom = dragScale(window, 62);
        DrawTextW(dc, L"Drag prepared files to Explorer · Copy only", -1, &rect, DT_CENTER | DT_SINGLELINE | DT_VCENTER | DT_END_ELLIPSIS);
        if (previous) SelectObject(dc, previous);
        if (font) DeleteObject(font);
        EndPaint(window, &paint);
        return 0;
    }
    case WM_ERASEBKGND: return 1;
    case WM_SETCURSOR:
        SetCursor(LoadCursorW(nullptr, view->files.empty() ? IDC_ARROW : IDC_HAND)); return TRUE;
    case WM_LBUTTONDOWN:
        if (!view->invalid && !view->dragging && !view->files.empty()) {
            view->pressed = true; view->origin = {GET_X_LPARAM(lParam), GET_Y_LPARAM(lParam)}; SetCapture(window);
        }
        return 0;
    case WM_MOUSEMOVE:
        if (view->pressed && (wParam & MK_LBUTTON)) {
            int dx = GET_X_LPARAM(lParam)-view->origin.x, dy = GET_Y_LPARAM(lParam)-view->origin.y;
            if (abs(dx) >= GetSystemMetrics(SM_CXDRAG) || abs(dy) >= GetSystemMetrics(SM_CYDRAG)) beginFileDrag(view);
        }
        return 0;
    case WM_LBUTTONUP:
        view->pressed = false;
        if (GetCapture() == window) ReleaseCapture();
        return 0;
    case WM_CAPTURECHANGED: case WM_CANCELMODE:
        view->pressed = false; return 0;
    case WM_NCDESTROY:
        view->window = nullptr;
        SetWindowLongPtrW(window, GWLP_USERDATA, 0);
        break;
    }
    return DefWindowProcW(window, message, wParam, lParam);
}

static LRESULT CALLBACK fileDragParent(HWND window, UINT message, WPARAM wParam, LPARAM lParam, UINT_PTR id, DWORD_PTR data) {
    auto view = reinterpret_cast<FileDragView *>(data);
    // Destruction dispatched by a nested OLE loop may release the Go owner.
    view->retain();
    if (message == WM_NCDESTROY) {
        RemoveWindowSubclass(window, fileDragParent, id);
        view->parent = nullptr;
    }
    LRESULT result = DefSubclassProc(window, message, wParam, lParam);
    if (message == WM_SIZE || message == WM_DPICHANGED || message == WM_WINDOWPOSCHANGED) positionFileDrag(view);
    view->release();
    return result;
}

extern "C" void *yd_file_drag_install(void *nativeWindow) {
    HWND parent = static_cast<HWND>(nativeWindow);
    if (!IsWindow(parent) || GetWindowThreadProcessId(parent, nullptr) != GetCurrentThreadId()) return nullptr;
    auto view = new(std::nothrow) FileDragView;
    if (!view) return nullptr;
    HRESULT hr = OleInitialize(nullptr);
    if (FAILED(hr)) { delete view; return nullptr; }
    view->ole = true; view->parent = parent;
    WNDCLASSW cls{};
    cls.lpfnWndProc = fileDragProcedure; cls.hInstance = GetModuleHandleW(nullptr);
    cls.lpszClassName = L"YourDeskFileDragSource";
    if (!RegisterClassW(&cls) && GetLastError() != ERROR_CLASS_ALREADY_EXISTS) { view->release(); return nullptr; }
    if (!CreateWindowExW(0, cls.lpszClassName, L"拖曳已下載檔案 / Drag downloaded files", WS_CHILD | WS_VISIBLE,
            0, 0, 0, 0, parent, nullptr, cls.hInstance, view)) { view->release(); return nullptr; }
    if (!SetWindowSubclass(parent, fileDragParent, reinterpret_cast<UINT_PTR>(view), reinterpret_cast<DWORD_PTR>(view))) {
        DestroyWindow(view->window); view->release(); return nullptr;
    }
    positionFileDrag(view);
    return view;
}

extern "C" int yd_file_drag_set(void *nativeView, const char **paths, int count) {
    auto view = static_cast<FileDragView *>(nativeView);
    if (!view || view->thread != GetCurrentThreadId() || view->invalid || !view->window || count < 0 || count > 64 || (count && !paths)) return 0;
    std::vector<std::wstring> files;
    try {
        files.reserve(count);
        for (int i = 0; i < count; ++i) {
            if (!paths[i]) return 0;
            int size = MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, paths[i], -1, nullptr, 0);
            if (size < 2 || size > 32767) return 0;
            std::wstring file(size, 0);
            if (!MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, paths[i], -1, &file[0], size)) return 0;
            file.resize(size-1);
            DWORD attributes = GetFileAttributesW(file.c_str());
            if (attributes == INVALID_FILE_ATTRIBUTES || (attributes & (FILE_ATTRIBUTE_DIRECTORY | FILE_ATTRIBUTE_REPARSE_POINT))) return 0;
            files.push_back(std::move(file));
        }
    } catch (...) { return 0; }
    view->files = std::move(files);
    view->pressed = false;
    InvalidateRect(view->window, nullptr, TRUE);
    return 1;
}

extern "C" void yd_file_drag_remove(void *nativeView) {
    auto view = static_cast<FileDragView *>(nativeView);
    if (!view || view->thread != GetCurrentThreadId()) return;
    view->invalid = true;
    if (view->parent) RemoveWindowSubclass(view->parent, fileDragParent, reinterpret_cast<UINT_PTR>(view));
    if (view->window) DestroyWindow(view->window);
    view->parent = nullptr;
    view->release();
}
