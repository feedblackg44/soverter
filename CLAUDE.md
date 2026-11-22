# vidpreset

Windows утилита на Go: ПКМ по видео → пресет ffmpeg → готовый файл рядом. Редактор пресетов (Fyne), окно прогресса с очередью, меню Explorer (новое Windows 11 и классическое). Личный проект, язык интерфейса и общения: русский.

## Команды

```powershell
go build -ldflags "-H=windowsgui" -o vidpreset.exe ./cmd/vidpreset   # GUI-сборка (нужен MinGW/gcc из-за Fyne, cgo)
go vet ./... ; go test ./...                                          # весь прогон ~90 с (e2e гоняют реальный ffmpeg)
go test ./internal/cmdbuild ./internal/ffimport ...                   # быстрые чистые пакеты
.\shellext\package.ps1                                                # пересобрать и подписать internal/shellext/assets/vidpreset.msix (только при правке ctx.cpp/манифеста)
```

Подкоманды exe: `edit` (по умолчанию), `run <id> <файл>...`, `pick`, `install`, `uninstall [--cert]`, `list`. GUI-сборка без консоли: вывод `list`/`install` виден только из обычного терминала (AttachConsole), из git-bash не виден.

## Архитектура (`internal/`)

- `preset` — `presets.toml` (`%APPDATA%\vidpreset`), валидация, умолчания. Пресет либо **структурный** (`[preset.video]`/`[preset.audio]`), либо **старый/сырой** (только `vf`/`af`/`args`). Старые обязаны работать без изменений. У структурных `vf`/`af`/`args` — дополнительные фильтры/аргументы.
- `cmdbuild` — чистая сборка аргументов ffmpeg и валидация (`Args`, `Validate`, `CheckAvailability`). Всё про флаги энкодеров живёт здесь и покрыто тестами по семействам × режимам.
- `encoders` — доступность энкодеров: софт по `-encoders`, железо пробным кодированием; кэш в памяти и на диске (`%LOCALAPPDATA%\vidpreset\encoders.json`, 7 дней).
- `sizecalc` — битрейт по целевому размеру из ffprobe. `runner` — запуск ffmpeg, два прохода, прогресс, очередь. `ffimport` — парсер «Импорт из команды».
- `ui` — Fyne: редактор (`editor.go`, `builder.go`), `picker.go`, `progress.go`. Логику сюда не класть.
- `shellint` — классическое меню (реестр HKCU, за интерфейсом для моков). `shellext` — меню Windows 11: подписанный MSIX с C++ COM-библиотекой (`/shellext`), список пресетов читает из `%APPDATA%\vidpreset\menu.txt`. `ipc` — single-instance через loopback-порт 47613.
- `e2e` — тесты на реальном ffmpeg/NVENC; пропускаются, если ffmpeg нет.

## Правила и грабли

- Пресеты и аргументы ffmpeg передаются массивом в `exec.Command`, без shell. Пути с пробелами и кириллицей должны работать.
- Не писать в `presets.toml` пользователя при тестах: подменяйте `APPDATA` на временную папку. Повреждённый конфиг никогда не перезаписывается.
- Меню Windows 11: нельзя регистрировать пакет из папок внутри `AppData` (рабочая папка — `%USERPROFILE%\.vidpreset\shellext`). `SystemFileAssociations\video` в этой системе не работает, классическое меню регистрируется по расширениям. Пакет подписан самоподписанным `CN=vidpreset`; доверие ставится через UAC, из сессии Claude этого делать нельзя (системная настройка).
- Не запускать `uninstall`/`install` на реальном реестре без нужды: это меню пользователя.
- В Bash-инструменте длинные heredoc и `perl -0pi` с `\n` в строках ломаются (портят `\n`, пути); для больших правок использовать Write/Edit.
- Поля TOML с нулём: числовые поля структурных пресетов помечены `omitempty,omitzero`, иначе пишутся `0.0`.
- AMF/QSV флаги собраны по документации, на железе не проверены (у владельца RTX 4070 SUPER).

## Git

Репозиторий локальный (ветка `main`). Коммит на логический этап, сообщения на английском, в конце строка `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`. Бинарники (`vidpreset.exe`) и `shellext/build/` в `.gitignore`; собранные `vidpreset.msix`/`.cer` в `internal/shellext/assets` коммитятся (вшиваются через go:embed).
