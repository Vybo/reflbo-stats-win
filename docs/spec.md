# reflbo-stats: design spec

- **Date:** 2026-10-19
- **Status:** Draft for the owner's review (r1)
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
- **Element sizes:** the reader takes them from the header, so a newer HWiNFO with longer elements still reads. It refuses a version it doesn't know, or sizes smaller than the fields it reads, and says so on the page.
- **States:**
  - **not running:** no mapping.
  - **shm off:** HWiNFO runs, the mapping is missing: Shared Memory Support is off in HWiNFO's settings.
  - **stale:** the poll time hasn't moved for 5 s.
  - **ok:** otherwise.
  - The board gets them in `/v1/info` and in `status` messages.
- **A reading's key:** sensor id, sensor instance and reading id, which stay the same across HWiNFO's restarts. The page shows HWiNFO's user labels where the owner renamed them in HWiNFO, else its original ones.
- **Text:** labels are read as UTF-8 where HWiNFO gives them (its `utf` fields), else from its ANSI ones in the system code page.
- **`-dump <file>`** writes the mapping as it is once, for tests. A dump holds sensor names and values only.

## 5. The picked readings

Each reading the owner ticks gets:

- **id:** 1–15 characters of a-z, 0-9 and _, made from its label (`cpu_package_temp` cut to `cpu_package_tem`, then made unique) and editable. The board's presets name it as `pc.<id>`, so renaming it breaks those.
- **label:** at most 23 bytes, HWiNFO's by default.
- **unit:** at most 7 bytes, HWiNFO's by default (`°C`, `%`, `MB`, `MHz`, `W`).
- **scale:** a factor applied before sending (1 by default; ÷1024 turns MB into GB), with the unit changed alongside.
- **decimals:** 0–3, by unit by default (0 for %, MHz and MB, 1 for °C, W and GB).
- **min and max:** the usual range the board's visuals start from. 0–100 for %; otherwise empty, and the board's charts go automatic.

At most 32 readings. Their order on the page is the catalogue's order, and the board's PC preset fills its slots from the first ones.

## 6. The LAN API (reflbo §12a)

- **Port** 47123 by default, on all interfaces, for `/v1/*` only. Every request needs `Authorization: Bearer <token>`, otherwise 401.
- `GET /v1/info` → `{"app":"reflbo-stats","protocol":1,"version":"<app version>","name":"<computer name>","hwinfo":"ok|not running|shm off|stale"}`.
- `GET /v1/stream` → WebSocket:
  - The board sends `hello` with its interval.
  - The app sends the `catalog`, then `readings` at that interval (clamped to 250 ms–10 s).
  - It sends the `catalog` again after any change on the page, and a `status` when HWiNFO's state changes.
  - It pings every 5 s.
  - `null` stands for a reading HWiNFO doesn't deliver now (stale, gone, or HWiNFO not running).
- **Limits:** frames of at most 8 KB each way; a board that sends more, or JSON deeper than 4 levels, is closed. At most 8 boards at once.
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
    - the LAN port (a restart applies it).
  - **App:** Start with Windows, the version, and the config file's path.
- **Windows firewall:** the first start asks to allow the app; private networks are enough. The page says so if no board has connected.

## 8. The tray

- **Left-click:** opens the settings page.
- **Right-click menu:**
  - **Open settings**;
  - **Start with Windows**, ticked when on;
  - **Quit**, which closes the boards' connections cleanly, so they end their sessions at once rather than after 15 s.
- **Icon:** shows the state: normal, a warning mark while HWiNFO isn't "ok", and a dot while a board is connected.
- **Auto-start:** off by default. When on, it is the per-user Run key (`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`) with the exe's path, so no administrator rights are needed. Turning it off removes the key.
- **One instance:** a second start opens the first one's page and exits.

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

A reading whose key HWiNFO no longer has (another CPU, a removed drive) stays in the list, marked missing, and is sent as `null`. Auto-start lives in the Run key, not in the file.

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
| A port is taken | The page and the log say which; both ports are configurable |

## 12. Licence

Apache-2.0, as reflbo. hwinfo-streamdeck (GPL-3.0) is a reference for what HWiNFO offers, never a source of code.

## 13. Revision history

| Rev | Date | Change |
|---|---|---|
| r1 | 2026-10-19 | First draft from the owner's brainstorming answers (reflbo D44) |
