// Standalone COM client: loads soverter_ctx.dll, enumerates the menu and prints titles.
#define UNICODE
#define _UNICODE
#include <windows.h>
#include <shobjidl.h>
#include <stdio.h>
static const CLSID CLSID_Soverter = {0x11e06287, 0x97a2, 0x497c, {0xa8, 0x58, 0x57, 0xdb, 0x8e, 0xe1, 0x51, 0x19}};
typedef HRESULT(STDAPICALLTYPE *GetCO)(REFCLSID, REFIID, void **);
int wmain(int argc, wchar_t **argv) {
    CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
    HMODULE h = LoadLibraryW(argv[1]);
    if (!h) { printf("LoadLibrary failed %lu\n", GetLastError()); return 1; }
    GetCO get = (GetCO)GetProcAddress(h, "DllGetClassObject");
    IClassFactory *f = nullptr;
    if (FAILED(get(CLSID_Soverter, IID_IClassFactory, (void **)&f))) { puts("no factory"); return 1; }
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
        while (en->Next(1, &c, &n) == S_OK && n == 1) { c->GetTitle(nullptr, &t); EXPCMDFLAGS cf; c->GetFlags(&cf); wprintf(L"  item: %ls (flags=%d)\n", t, (int)cf); c->Release(); }
        en->Release();
    }
    root->Release(); f->Release();
    return 0;
}
