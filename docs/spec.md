# reflbo-stats: design spec

- **Date:** 2026-10-19
- **Status:** As built (r3, after the branch's review); the owner's checks on the PC wait (§10)
- **Covers:** the Windows tray app that serves a PC's HWiNFO sensors to [reflbo](https://github.com/Vybo/reflbo) boards on the LAN
- **Related:** reflbo's design spec, `docs/specs/2026-09-25-firmware-design.md` in the reflbo repository: §12a (the protocol and the board's side), decision D44 (§1.2), milestone M7 (§18). Where the two disagree on the protocol, reflbo's §12a wins; fix this file.

## 1. Purpose

reflbo is a desk display. While the owner works at their Windows PC, it should show the PC's load and temperatures live, as numbers, charts, bars and gauges, and go back to its usual dashboards when the PC sleeps. This app is the PC's half: it reads the sensors HWiNFO measures and serves the ones the owner picks to reflbo boards on the home network.

Owner decisions (2026-10-19, reflbo D44):

- HWiNFO **Pro**, read through its **shared memory**. The free HWiNFO stops shared memory after 12 h, and its registry ("Gadget") interface isn't used.
- The sensors are picked in **this app**, on a page of its own.
- **Go**, one `.exe`, a tray icon and a local web page, built and tested from the owner's Mac.
- **WebSocket** to the board, with a token.
- The board sets the interval (250 ms–10 s, default 1 s).
- Units and scale are set **here** (MB → GB, for example).
- The tray's right-click menu has **Start with Windows** (auto-start after login, off by default) and **Quit**.

## 2. Scope

In: one PC, Windows 10 and 11 (x64), HWiNFO Pro 7.x and later; several boards at once; a demo source for tests.

Out: macOS or Linux as real sources (the demo source builds there, for tests only); HWiNFO's registry interface; other sources (LibreHardwareMonitor, AIDA64); signing the exe; an installer; anything but reading and serving sensors.

## 3. Architecture

```
cmd/reflbo-stats/   main: flags, the tray, wiring
internal/hwinfo/    the shared-memory reader: header, sensors, readings, states   [tests on dumps]
internal/source/    the Source interface; HWiNFO (Windows) and Demo (all builds)
internal/config/    config.json: the picked readings, the token, the ports, auto-start
internal/server/    the LAN API (/v1/info, /v1/stream) and the settings page's API     [tests with httptest]
internal/autostart/ the per-user Run key (Windows; a no-op elsewhere)
web/                the settings page: plain HTML, CSS and JS, embedded               [tests in Node]
testdata/           HWiNFO dumps made with -dump on the owner's PC
```

- **One loop** reads the source at the fastest interval a connected board asks for, never faster than 250 ms, and with no board at 1 s for the settings page. Each board gets the newest values at its own interval.
- **Dependencies:** `fyne.io/systray` (Apache-2.0), `github.com/coder/websocket` (ISC), `golang.org/x/sys/windows` (BSD-3). Nothing else beyond the standard library.
- **Builds:** `GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui"` from the Mac. The demo source makes the same binary run on macOS for tests.

## 4. Reading HWiNFO

- **Where:** the file mapping `Global\HWiNFO_SENS_SM2`, opened read-only (`OpenFileMapping`, `MapViewOfFile`), as HWiNFO's published shared-memory header (`hwisenssm2.h`) describes it. The header is signature `HWiS`, version, revision, poll time, and the offsets, sizes and counts of the sensor and reading sections. Each reading has its type, sensor index, reading id, labels, unit, and value, minimum, maximum and average.
- **Element sizes:** the reader takes them from the header, so a newer HWiNFO with longer elements still reads. It refuses a version it doesn't know (as built it knows 1 and 2), or sizes smaller than the fields it reads, and says so on the page. The boards then see "not running" (the protocol's four states have no fifth), while the page shows the reason.
- **States:**
  - **not running:** no mapping.
  - **shm off:** HWiNFO runs, the mapping is missing: Shared Memory Support is off in HWiNFO's settings.
  - **stale:** the poll time hasn't moved for three of HWiNFO's polling periods (the header's, offset 44), and never under 5 s, as the poll time counts whole seconds.
  - **ok:** otherwise.
  - The board gets them in `/v1/info` and in `status` messages.
- **A reading's key:** sensor id, sensor instance and reading id, which stay the same across HWiNFO's restarts. The page shows HWiNFO's user labels where the owner renamed them in HWiNFO, else its original ones.
- **Text:** labels are read as UTF-8 where HWiNFO gives them (its `utf` fields), else from its ANSI ones in the system code page.
- **`-dump <file>`** writes the mapping as it is once, for tests. A dump holds sensor names and values only.
- **Reading it** (as built): under HWiNFO's mutex (`Global\HWiNFO_SM2_MUTEX`, 50 ms at most; an abandoned one counts as taken) when it has one, so a poll in progress isn't read half-written, with the goroutine locked to its OS thread meanwhile, as a Windows mutex belongs to its thread; "shm off" against "not running" by whether an `hwinfo*.exe` process runs.
- **Numbers:** a value, minimum or maximum that isn't finite (HWiNFO's NaN, an infinity) is no value (`null`), as JSON can't carry it; so is a value a scale overflows.

## 5. The picked readings

Each reading the owner ticks gets:

- **id:** 1–15 characters of a-z, 0-9 and _, made from its label (`cpu_package_temp` cut to `cpu_package_tem`, then made unique) and editable. The board's presets name it as `pc.<id>`, so renaming it breaks those.
- **label:** at most 23 bytes, HWiNFO's by default.
- **unit:** at most 7 bytes, HWiNFO's by default (`°C`, `%`, `MB`, `MHz`, `W`).
- **scale:** a factor applied before sending (1 by default; ÷1024 turns MB into GB), with the unit changed alongside.
- **decimals:** 0–3, by unit by default (0 for %, MHz and MB, 1 for °C, W and GB).
- **min and max:** the usual range the board's visuals start from. 0–100 for %; otherwise empty, and the board's charts go automatic.

At most 32 readings. Their order on the page is the catalogue's order, and the board's PC preset fills its slots from the first ones.

As built: a newly ticked reading goes to the app with its key alone, and the app fills in the rest on Save, so its rules live in one place: the label and unit from HWiNFO (cut at a character), the id from the label, the scale 1, decimals 0 for %, MHz, MB, RPM, KB/s and MB/s, 3 for V and 1 for the rest, and 0–100 for a %. The scale is typed as a number or a fraction (`1/1024`) and shown back as one.

## 6. The LAN API (reflbo §12a)

- **Port** 47123 by default, on all interfaces, for `/v1/*` only. Every request needs `Authorization: Bearer <token>`, otherwise 401.
- `GET /v1/info` → `{"app":"reflbo-stats","protocol":1,"version":"<app version>","name":"<computer name>","hwinfo":"ok|not running|shm off|stale"}`.
- `GET /v1/stream` → WebSocket:
  - The board sends `hello` with its interval.
  - The app sends the `catalog`, then `readings` at that interval (clamped to 250 ms–10 s).
  - It sends the `catalog` again after any change on the page, and a `status` when HWiNFO's state changes.
  - It pings every 5 s.
  - `null` stands for a reading HWiNFO doesn't deliver now (stale, gone, or HWiNFO not running).
- **Limits:** frames of at most 8 KB each way; a board that sends more, or JSON deeper than 4 levels, is closed. At most 8 boards at once; a ninth gets 503.
- **Closing** (as built), with the reason a board sees: "no hello" after 5 s without one, "protocol N", "too deep" and "new token" (policy violation, 1008), a frame over 8 KB (1009), and Quit (going away, 1001).
- **The source** is read at the fastest interval a connected board asked for, never below 250 ms, and once a second with no board.
- **A LAN port the app can't open** (another program holds it) doesn't stop it: the settings page and the tray run, the page says why under Connection (and opens by itself at a start), and Apply on a new port opens it at once.
- **Token:** 128 random bits, shown as 26 base32 characters in groups. "New token" on the page closes every board's connection.

## 7. The settings page

- **Where:** `http://127.0.0.1:47124/`, on the loopback interface only, so no one on the LAN can change it. The tray opens it in the default browser.
- **Sections:**
  - **HWiNFO:** its state with what to do: start HWiNFO, turn on Shared Memory Support, or see the header's version.
  - **Readings:** a search box and HWiNFO's sensors as groups, each reading with its live value and a tick. A ticked reading shows its id, label, unit, scale, decimals, min and max, and can move up or down. A save applies at once and sends the boards the new catalogue.
  - **Boards:** each connected board's address, interval and connection time.
  - **Connection:**
    - the PC's address(es) and port, to enter on reflbo's PC page;
    - the token, with Copy and New token;
    - the LAN port (Apply opens it at once).
  - **App:** Start with Windows, the LAN port (Apply opens it at once), the version, and the config file's path.
- **As built:**
  - the live values refresh every second without redrawing the fields being edited; an edit updates Save and Undo in place, so the focus stays and the first click saves; the search redraws the list under its box, and a group the owner collapsed stays collapsed;
  - New token takes a second click to confirm (no `confirm()`, so a driven browser doesn't block);
  - Save and Undo changes work on the whole list;
  - a refusal shows the app's reason and keeps the edits.
- **Windows firewall:** the first start asks to allow the app; private networks are enough. The page says so if no board has connected.

## 8. The tray

- **Left-click:** opens the settings page.
- **Right-click menu:**
  - **Open settings**;
  - **Start with Windows**, ticked when on;
  - **Quit**, which closes the boards' connections cleanly, so they end their sessions at once rather than after 15 s.
- **Icon:** shows the state: normal, a warning mark while HWiNFO isn't "ok", and a dot while a board is connected.
- **Auto-start:** off by default. When on, it is the per-user Run key (`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`) with the exe's path, so no administrator rights are needed. Turning it off removes the key.
- **One instance:** a second start opens the first one's page and exits. As built, the settings page's port tells: a second app can't listen on it.
- **The first start**, with no reading picked, opens the settings page.
- **The tooltip** reads HWiNFO's state and the number of boards; the menu's Start with Windows follows the page's checkbox within 2 s.

## 9. Configuration

`%APPDATA%\reflbo-stats\config.json`, written atomically (a temporary file, then a rename), with a `.bak` of the previous version:

```json
{
  "schema": 1,
  "port": 47123,
  "ui_port": 47124,
  "token": "…",
  "readings": [
    { "key": "0xf0000400:0:0x1000000", "id": "cpu_load", "label": "CPU load", "unit": "%", "scale": 1, "decimals": 0, "min": 0, "max": 100 }
  ]
}
```

A reading whose key HWiNFO no longer has (another CPU, a removed drive) stays in the list, marked missing, and is sent as `null`. Auto-start lives in the Run key, not in the file. A missing or broken file and `.bak` give the defaults, written at once. The log is `log.txt` beside it, started afresh past 1 MB; the exe has no console.

## 10. Tests and checks

- **Go tests on the Mac:**
  - the reader on the owner's dumps (sizes, states, labels, a newer element size, a truncated mapping);
  - the server (401, `/v1/info`, the stream's catalogue, readings, intervals, `null`s, limits, 8 boards, Quit's close);
  - the config file (atomic write, `.bak`, ids);
  - the demo source.
- **The page:** Node's test runner against a fake API, as reflbo's page tests run, and screenshots in headless Chrome.
- **The exe:** cross-compiled, and its size checked.
- **With a board:** the demo source on the Mac serves a real reflbo board, which finds it at a sync and draws it, as reflbo's M7 plan checks.
- **On the owner's PC:**
  1. it starts, and the firewall prompt appears;
  2. HWiNFO Pro is found;
  3. the picker lists the sensors;
  4. auto-start survives a reboot;
  5. Quit ends a board's session at once;
  6. a `-dump` for the tests.

## 11. Risks

| Risk | Mitigation |
|---|---|
| HWiNFO changes its shared memory | The header's sizes are honoured; an unknown version is refused and named on the page |
| The unsigned exe trips SmartScreen; an antivirus may flag an exe that reads another process's shared memory | Warn in the README; build reproducibly from the source |
| The token travels in plain HTTP on the LAN | LAN only; New token on the page |
| A port is taken | The LAN port: the page and the log say which, and another opens at once; the settings page's: a second copy takes it for the first one running, and it is set in config.json |

## 12. Open items

Minors from the branch's review (2026-10-19), to fix where later work touches the code:
- Nothing keeps the app's own frames under 8 KB: labels full of `<`, `>` or `&` are escaped to 6 bytes each (`SetEscapeHTML(false)` and a size check on Save).
- A new token or Quit closes the boards one after another, 5 s each for one that doesn't answer; the LAN listener should stop first, and the closes run at once.
- The ping waits in the write loop, delaying the readings by a round trip every 5 s.
- A hello without `interval_ms` gets 250 ms, not 1 s; `1000.0` doesn't parse.
- The source's loop and each board's ticker aren't in step, so a value can be an interval old or repeat; a board before its hello slows the loop to 10 s.
- A fifth interval change in a row is dropped (a queue of four).
- Any settings change resends an unchanged catalogue.
- The config file is two renames, not one replace (`ReplaceFileW`), and Defender's sharing violations aren't retried; the page's API doesn't serialise its read-modify-write of it.
- ANSI text reads as Latin-1, not the system code page (cp1250 letters); a UTF-8 field cut mid-character falls back whole.
- The scale field shows a bad input back as NaN (Save is blocked with a message, and a decimal comma works); `maxlength` counts characters where the app counts bytes.
- The Origin check takes any loopback port; an oversized body gets "send a list of readings", not 413.
- Every Windows session shares loopback: another logged-in user's browser reaches this user's page.
- Start with Windows stays ticked when Task Manager has disabled the entry (`StartupApproved\Run`).
- The addresses include virtual adapters (WSL, Hyper-V, VPN); a board before its hello lists "0 ms".
- No test covers a half-open or slow board, or the page's focus (the fake DOM has none).
- A stale refresh that arrives after a Save can put the pre-save list back for about a second, and an edit made in that second saves over the Save (needs a request generation counter).
- Every hello makes the app read HWiNFO again, so a token holder that loops hellos makes it contend for HWiNFO's mutex.
- A board closed by New token while its upgrade is in flight gets 1001 with no reason (the board shows "the PC app quit"; the session ends either way).
- A port-open failure other than "address in use" (WSAEACCES from Hyper-V's or WSL's reserved ranges) ends the windowed app through `logger.Fatal` with nothing on screen; it is in `log.txt` now.
- A failed port change shows the port as "—" and "boards can't reach this PC" while the old listener still serves the boards.

## 13. Licence

GPL-3.0-only, as reflbo (its D46, 2026-10-21); versions up to `fef9fa7` stay Apache-2.0. The dependencies keep their own licences (§3, `NOTICE`). hwinfo-streamdeck (GPL-3.0) is a reference for what HWiNFO offers, never a source of code.

## 14. Revision history

| Rev | Date | Change |
|---|---|---|
| r1 | 2026-10-19 | First draft from the owner's brainstorming answers (reflbo D44) |
| r2 | 2026-10-19 | As built on `plan/m7`: the known layouts and their refusals (§4), defaults filled by the app (§5), closing reasons and the source's pace (§6), the page and the tray as built (§7, §8), the log (§9) |
| r3 | 2026-10-19 | The branch's review: stale by HWiNFO's polling period, non-finite numbers as none, the mutex's thread (§4); a taken LAN port (§6, §11); the page's edits in place (§7); the open items (§12) |
| r4 | 2026-10-20 | The final review's fixes: the log file first, the polling period only from a header that has it, a null scale refused, the scale's comma and its message, sensors appearing after the page opened, the decimals a new reading shows; the Connection wording (§7) and the new open items (§12) |
| r5 | 2026-10-21 | The licence: GPL-3.0-only, as reflbo (its D46) (§13) |
