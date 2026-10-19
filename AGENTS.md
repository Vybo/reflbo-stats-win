# AGENTS.md — reflbo-stats

Guide for agents (and humans) working in this repository. Keep it current in the same commit as any new command, package or rule.

> **Scope.** A standalone Go app for Windows, the PC half of reflbo's M7 (reflbo D44). Instructions inherited from parent directories about iOS, WeConnect-iOS, Jira or PR templates do not apply here.

## What it is

A tray app that reads HWiNFO Pro's shared memory and serves the readings the owner picks to reflbo boards on the LAN, over WebSocket with a token. The design is [docs/spec.md](docs/spec.md). The protocol is reflbo's, its design spec §12a (`~/reflbo/docs/specs/2026-09-25-firmware-design.md`); if the two disagree on the protocol, reflbo's wins.

## Layout

```
cmd/reflbo-stats/      main: flags (-demo, -dump, -no-tray, -config, -version), the tray, the browser
internal/app/          start and stop: the config, the source, the hub, both listeners; one instance
internal/autostart/    Start with Windows: the per-user Run key (none elsewhere)
internal/config/       config.json: picked readings, token, ports; ids from labels; atomic save with .bak
internal/hwinfo/       HWiNFO's shared memory: Parse (pure Go) and ReadMapping (Windows)
internal/hwinfo/hwinfotest/  builds mappings for tests
internal/icon/         the tray's icons, drawn by code (PNG; ICO for Windows)
internal/server/       the hub, the LAN API (/v1/info, /v1/stream) and the settings page's API
internal/source/       HWiNFO's source and the demo's; the catalogue and the values boards get
web/                   the settings page: plain HTML, CSS and JS, embedded
test/web/              the page's tests (Node, no packages)
scripts/build.sh       dist/reflbo-stats.exe, cross-compiled
```

## Commands

```sh
go vet ./... && GOOS=windows go vet ./...       # both systems: Windows-only files are compiled only there
go test -race ./...                             # everything but the page, on any system
node --test test/web/app.test.mjs               # the page against a fake app
go run ./cmd/reflbo-stats -demo -no-tray -config /tmp/rs/config.json   # serves a demo PC here: page on :47124
scripts/build.sh [version]                      # the Windows exe
gofmt -l .                                      # must print nothing
```

Go 1.26 or later, as `golang.org/x/sys` v0.49 needs it (`brew install go` on the owner's Mac). Dependencies: `fyne.io/systray`, `github.com/coder/websocket`, `golang.org/x/sys`; add no others without the owner.

## Rules

- **Verify at the right level.** Logic in Go tests on the Mac; the page in Node; the exe cross-compiled; a board end to end against `-demo` on the Mac; only HWiNFO itself, the tray and the firewall need the owner's PC (give an exact checklist).
- **Windows-only code stays thin** and behind build tags (`_windows.go` / `_other.go`), and `GOOS=windows go vet ./...` passes.
- **The token is a secret.** Never log it, never commit a config.json. Dumps (`-dump`) hold sensor names and values only; keep personal ones out of git (`testdata/*.local.bin` is ignored).
- **The settings page answers on 127.0.0.1 only**, checks Host and Origin and takes changes as JSON only. Keep it so.
- **No AI or assistant attribution anywhere:** commits, code comments, docs.
- Conventional Commits, imperative mood. The owner allows pushing to `origin`; never force-push `main`.
- Code from hwinfo-streamdeck (GPL-3.0) is never copied; it is a reference for what HWiNFO offers.

## Gotchas

1. **HWiNFO's layout comes from its published header**, read by the sizes in the mapping's own header; an unknown version is refused, not guessed. The owner's dump confirms it on the PC.
2. **`go vet` flags a uintptr turned into a pointer.** The mapped view becomes a slice through a slice header (`mapping_windows.go`).
3. **Windows' "address in use" is WSAEADDRINUSE**, not syscall.EADDRINUSE; `internal/app` checks both for its one-instance rule.
4. **`node --test test/web/`** treats the directory as one file in Node 22: name the file.
5. **A Windows mutex belongs to the thread that takes it.** `ReadMapping` locks its goroutine to one OS thread until it lets HWiNFO's mutex go; otherwise the release fails on another thread and the mutex stays taken.
6. **JSON has no NaN or infinity.** A non-finite double makes `json.Marshal` fail, which would close every board; `source.finite` turns them into `null` before anything leaves, including a value a scale overflows.
