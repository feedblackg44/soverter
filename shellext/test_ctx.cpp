// Standalone COM client: loads vidpreset_ctx.dll, enumerates the menu and prints titles.
#define UNICODE
#define _UNICODE
#include <windows.h>
#include <shobjidl.h>
#include <stdio.h>
static const CLSID CLSID_VidPreset = {0x5c1d2a7e, 0x9b3f, 0x4e6a, {0x8d, 0x41, 0x7a, 0x2f, 0x0b, 0x6c, 0x9e, 0x13}};
typedef HRESULT(STDAPICALLTYPE *GetCO)(REFCLSID, REFIID, void **);
int wmain(int argc, wchar_t **argv) {
    CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
    HMODULE h = LoadLibraryW(argv[1]);
    if (!h) { printf("LoadLibrary failed %lu\n", GetLastError()); return 1; }
    GetCO get = (GetCO)GetProcAddress(h, "DllGetClassObject");
    IClassFactory *f = nullptr;
    if (FAILED(get(CLSID_VidPreset, IID_IClassFactory, (void **)&f))) { puts("no factory"); return 1; }
    IExplorerCommand *root = nullptr;
    if (FAILED(f->CreateInstance(nullptr, IID_IExplorerCommand, (void **)&root))) { puts("no instance"); return 1; }
    LPWSTR t = nullptr; root->GetTitle(nullptr, &t); wprintf(L"root len=%d:", (int)wcslen(t));
    for (int i = 0; t[i] && i < 8; i++) wprintf(L" %04x", t[i]);
    wprintf(L"\n");
    EXPCMDSTATE st; root->GetState(nullptr, FALSE, &st); EXPCMDFLAGS fl; root->GetFlags(&fl);
    wprintf(L"state=%d flags=%d\n", (int)st, (int)fl);
    IEnumExplorerCommand *en = nullptr;
    if (SUCCEEDED(root->EnumSubCommands(&en))) {
        IExplorerCommand *c; ULONG n;
        while (en->Next(1, &c, &n) == S_OK && n == 1) { c->GetTitle(nullptr, &t); wprintf(L"  item: %s\n", t); c->Release(); }
        en->Release();
    }
    root->Release(); f->Release();
    return 0;
}
