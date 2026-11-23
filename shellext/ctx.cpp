// Soverter Windows 11 context menu handler (IExplorerCommand).
// Reads %APPDATA%\soverter\menu.txt (UTF-8):
//   exe=<full path to soverter.exe>
//   mode=submenu|single
//   title=<name of the root menu item>   (optional; default follows the Windows display language)
//   <id>\t<name>      (one line per enabled preset, in menu order)
// and builds a submenu with that title. Invoking an entry runs: exe run <id> <files...>
#define UNICODE
#define _UNICODE
#include <windows.h>
#include <shobjidl.h>
#include <shlobj.h>
#include <shlwapi.h>
#include <string>
#include <vector>
#include <new>

// {11E06287-97A2-497C-A858-57DB8EE15119}
static const CLSID CLSID_Soverter = {0x11e06287, 0x97a2, 0x497c, {0xa8, 0x58, 0x57, 0xdb, 0x8e, 0xe1, 0x51, 0x19}};

static LONG g_objects = 0;
static LONG g_locks = 0;

struct Entry {
    std::wstring id, name;
};
struct MenuData {
    std::wstring exe;
    bool single = false;
    std::wstring title;
    std::vector<Entry> items;
};

// Default root title in the style of "Edit with Notepad++", chosen by the Windows display language.
static const wchar_t *DefaultTitle() {
    return (GetUserDefaultUILanguage() & 0x3FF) == LANG_RUSSIAN ? L"Обработать с помощью Soverter" : L"Convert with Soverter";
}

static std::wstring Widen(const std::string &s) {
    if (s.empty()) return L"";
    int n = MultiByteToWideChar(CP_UTF8, 0, s.data(), (int)s.size(), nullptr, 0);
    std::wstring w(n, L'\0');
    MultiByteToWideChar(CP_UTF8, 0, s.data(), (int)s.size(), &w[0], n);
    return w;
}

static bool LoadMenu(MenuData &m) {
    PWSTR dir = nullptr;
    if (FAILED(SHGetKnownFolderPath(FOLDERID_RoamingAppData, 0, nullptr, &dir))) return false;
    std::wstring path = std::wstring(dir) + L"\\soverter\\menu.txt";
    CoTaskMemFree(dir);
    HANDLE h = CreateFileW(path.c_str(), GENERIC_READ, FILE_SHARE_READ | FILE_SHARE_WRITE, nullptr, OPEN_EXISTING,
                           FILE_ATTRIBUTE_NORMAL, nullptr);
    if (h == INVALID_HANDLE_VALUE) return false;
    std::string data;
    char buf[4096];
    DWORD got;
    while (ReadFile(h, buf, sizeof buf, &got, nullptr) && got) data.append(buf, got);
    CloseHandle(h);
    if (data.size() >= 3 && (unsigned char)data[0] == 0xEF && (unsigned char)data[1] == 0xBB) data.erase(0, 3);

    size_t pos = 0;
    while (pos < data.size()) {
        size_t nl = data.find('\n', pos);
        if (nl == std::string::npos) nl = data.size();
        std::string line = data.substr(pos, nl - pos);
        pos = nl + 1;
        while (!line.empty() && (line.back() == '\r')) line.pop_back();
        if (line.empty()) continue;
        if (line.rfind("exe=", 0) == 0) {
            m.exe = Widen(line.substr(4));
        } else if (line.rfind("mode=", 0) == 0) {
            m.single = line.substr(5) == "single";
        } else if (line.rfind("title=", 0) == 0) {
            m.title = Widen(line.substr(6));
        } else {
            size_t tab = line.find('\t');
            if (tab == std::string::npos) continue;
            m.items.push_back({Widen(line.substr(0, tab)), Widen(line.substr(tab + 1))});
        }
    }
    return !m.exe.empty();
}

// Launch `exe args... files...`.
static HRESULT Launch(const std::wstring &exe, const std::wstring &verb, const std::wstring &id, IShellItemArray *items) {
    if (!items) return E_INVALIDARG;
    DWORD n = 0;
    items->GetCount(&n);
    std::wstring cmd = L"\"" + exe + L"\" " + verb;
    if (!id.empty()) cmd += L" " + id;
    for (DWORD i = 0; i < n; i++) {
        IShellItem *it = nullptr;
        if (FAILED(items->GetItemAt(i, &it))) continue;
        PWSTR p = nullptr;
        if (SUCCEEDED(it->GetDisplayName(SIGDN_FILESYSPATH, &p))) {
            cmd += L" \"" + std::wstring(p) + L"\"";
            CoTaskMemFree(p);
        }
        it->Release();
    }
    STARTUPINFOW si = {sizeof si};
    PROCESS_INFORMATION pi = {};
    std::vector<wchar_t> mut(cmd.begin(), cmd.end());
    mut.push_back(L'\0');
    if (!CreateProcessW(nullptr, mut.data(), nullptr, nullptr, FALSE, 0, nullptr, nullptr, &si, &pi))
        return HRESULT_FROM_WIN32(GetLastError());
    CloseHandle(pi.hThread);
    CloseHandle(pi.hProcess);
    return S_OK;
}

enum Kind { ROOT, PRESET };

class Command : public IExplorerCommand {
    LONG ref_ = 1;
    Kind kind_;
    MenuData menu_;
    Entry entry_;

public:
    Command(Kind k, const MenuData &m, const Entry &e = Entry()) : kind_(k), menu_(m), entry_(e) { InterlockedIncrement(&g_objects); }
    ~Command() { InterlockedDecrement(&g_objects); }

    // IUnknown
    STDMETHODIMP QueryInterface(REFIID riid, void **ppv) override {
        if (riid == IID_IUnknown || riid == IID_IExplorerCommand) {
            *ppv = static_cast<IExplorerCommand *>(this);
            AddRef();
            return S_OK;
        }
        *ppv = nullptr;
        return E_NOINTERFACE;
    }
    STDMETHODIMP_(ULONG) AddRef() override { return InterlockedIncrement(&ref_); }
    STDMETHODIMP_(ULONG) Release() override {
        LONG r = InterlockedDecrement(&ref_);
        if (r == 0) delete this;
        return r;
    }

    // IExplorerCommand
    STDMETHODIMP GetTitle(IShellItemArray *, LPWSTR *name) override {
        return SHStrDupW(kind_ == ROOT ? (menu_.title.empty() ? DefaultTitle() : menu_.title.c_str()) : entry_.name.c_str(), name);
    }
    STDMETHODIMP GetIcon(IShellItemArray *, LPWSTR *icon) override {
        *icon = nullptr;
        return E_NOTIMPL;
    }
    STDMETHODIMP GetToolTip(IShellItemArray *, LPWSTR *tip) override {
        *tip = nullptr;
        return E_NOTIMPL;
    }
    STDMETHODIMP GetCanonicalName(GUID *g) override {
        *g = GUID_NULL;
        return E_NOTIMPL;
    }
    STDMETHODIMP GetState(IShellItemArray *, BOOL, EXPCMDSTATE *state) override {
        *state = ECS_ENABLED;
        if (kind_ == ROOT && !menu_.single && menu_.items.empty()) *state = ECS_HIDDEN;
        return S_OK;
    }
    STDMETHODIMP Invoke(IShellItemArray *items, IBindCtx *) override {
        if (kind_ == PRESET) return Launch(menu_.exe, L"run", entry_.id, items);
        if (menu_.single) return Launch(menu_.exe, L"pick", L"", items);
        return S_OK;
    }
    STDMETHODIMP GetFlags(EXPCMDFLAGS *flags) override {
        *flags = (kind_ == ROOT && !menu_.single) ? ECF_HASSUBCOMMANDS : ECF_DEFAULT;
        return S_OK;
    }
    STDMETHODIMP EnumSubCommands(IEnumExplorerCommand **e) override;
};

class Enum : public IEnumExplorerCommand {
    LONG ref_ = 1;
    MenuData menu_;
    size_t idx_ = 0;

public:
    explicit Enum(const MenuData &m) : menu_(m) { InterlockedIncrement(&g_objects); }
    ~Enum() { InterlockedDecrement(&g_objects); }
    STDMETHODIMP QueryInterface(REFIID riid, void **ppv) override {
        if (riid == IID_IUnknown || riid == IID_IEnumExplorerCommand) {
            *ppv = static_cast<IEnumExplorerCommand *>(this);
            AddRef();
            return S_OK;
        }
        *ppv = nullptr;
        return E_NOINTERFACE;
    }
    STDMETHODIMP_(ULONG) AddRef() override { return InterlockedIncrement(&ref_); }
    STDMETHODIMP_(ULONG) Release() override {
        LONG r = InterlockedDecrement(&ref_);
        if (r == 0) delete this;
        return r;
    }
    STDMETHODIMP Next(ULONG celt, IExplorerCommand **out, ULONG *fetched) override {
        ULONG n = 0;
        while (n < celt && idx_ < menu_.items.size()) {
            out[n++] = new (std::nothrow) Command(PRESET, menu_, menu_.items[idx_++]);
        }
        if (fetched) *fetched = n;
        return n == celt ? S_OK : S_FALSE;
    }
    STDMETHODIMP Skip(ULONG celt) override {
        idx_ += celt;
        return idx_ <= menu_.items.size() ? S_OK : S_FALSE;
    }
    STDMETHODIMP Reset() override {
        idx_ = 0;
        return S_OK;
    }
    STDMETHODIMP Clone(IEnumExplorerCommand **e) override {
        Enum *c = new (std::nothrow) Enum(menu_);
        if (!c) return E_OUTOFMEMORY;
        c->idx_ = idx_;
        *e = c;
        return S_OK;
    }
};

STDMETHODIMP Command::EnumSubCommands(IEnumExplorerCommand **e) {
    *e = nullptr;
    if (kind_ != ROOT || menu_.single) return E_NOTIMPL;
    Enum *en = new (std::nothrow) Enum(menu_);
    if (!en) return E_OUTOFMEMORY;
    *e = en;
    return S_OK;
}

class Factory : public IClassFactory {
    LONG ref_ = 1;

public:
    STDMETHODIMP QueryInterface(REFIID riid, void **ppv) override {
        if (riid == IID_IUnknown || riid == IID_IClassFactory) {
            *ppv = static_cast<IClassFactory *>(this);
            AddRef();
            return S_OK;
        }
        *ppv = nullptr;
        return E_NOINTERFACE;
    }
    STDMETHODIMP_(ULONG) AddRef() override { return InterlockedIncrement(&ref_); }
    STDMETHODIMP_(ULONG) Release() override {
        LONG r = InterlockedDecrement(&ref_);
        if (r == 0) delete this;
        return r;
    }
    STDMETHODIMP CreateInstance(IUnknown *outer, REFIID riid, void **ppv) override {
        *ppv = nullptr;
        if (outer) return CLASS_E_NOAGGREGATION;
        MenuData m;
        LoadMenu(m); // missing file -> empty menu, root command hides itself
        Command *c = new (std::nothrow) Command(ROOT, m);
        if (!c) return E_OUTOFMEMORY;
        HRESULT hr = c->QueryInterface(riid, ppv);
        c->Release();
        return hr;
    }
    STDMETHODIMP LockServer(BOOL lock) override {
        if (lock) InterlockedIncrement(&g_locks);
        else InterlockedDecrement(&g_locks);
        return S_OK;
    }
};

extern "C" HRESULT STDAPICALLTYPE DllGetClassObject(REFCLSID rclsid, REFIID riid, void **ppv) {
    *ppv = nullptr;
    if (!IsEqualCLSID(rclsid, CLSID_Soverter)) return CLASS_E_CLASSNOTAVAILABLE;
    Factory *f = new (std::nothrow) Factory();
    if (!f) return E_OUTOFMEMORY;
    HRESULT hr = f->QueryInterface(riid, ppv);
    f->Release();
    return hr;
}

extern "C" HRESULT STDAPICALLTYPE DllCanUnloadNow() { return (g_objects == 0 && g_locks == 0) ? S_OK : S_FALSE; }
