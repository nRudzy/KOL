# KOL — journal de presse-papiers Windows

## Intent
An unobtrusive Windows 10/11 desktop utility for Gabriel: every copy remains locally available, searchable and restorable from the notification area. User authorizes implementation, logo generation, and publishing a new public GitHub repository named KOL. Implementation decisions are delegated to the builder.

## Behavior
- Listen to Windows clipboard updates, including copies outside KOL. Left tray click toggles a panel near the taskbar; right click opens a menu. Ctrl+Alt+V opens the panel. Escape/outside click hides it; closing never quits the background listener.
- Preserve raw supported clipboard bytes, including whitespace and line endings. Preview formatting never changes stored content. Deduplicate by a stable SHA-256 identity of named content formats, independent of enumeration order or session-specific registered IDs. Same text with different rich formatting remains distinct. Copies update recency and count.
- Capture Unicode/ANSI text, URLs, HTML, RTF, DIB/DIBV5/PNG/JPEG images, file lists, enhanced metafiles and registered HGLOBAL formats. Restore all captured formats in original priority order. Register custom names again after reboot. File lists refer to original files, not backups. Always restore filesystem operations as copy, never replay a destructive cut/move.
- Do not promise universal serialization: live COM streams/virtual files, application-private handles and owner-rendered data can depend on a running source application. Show partial-capture warnings. Respect explicit application signals excluding clipboard monitoring/password contents.
- Search and filters: all, text (including links), images, files, pinned. Click or Enter restores a copy, then the user pastes with Ctrl+V. Pin, delete, clear unpinned, pause capture, opt-in start with Windows.
- No count/time retention by default; 64 MiB per captured item by default, adjustable. Surface oversized/unavailable formats and storage failures. No network requests, accounts, telemetry or external resources in the running app.
- Store under `%LOCALAPPDATA%\KOL`. AES-256-GCM encrypts both metadata and payload; Windows DPAPI protects the local key. Concurrent instances focus the existing panel. Only one writer owns the local database.

## Architecture and approaches
Builder's choice: Go + direct Win32 APIs, bbolt transactions, and local embedded HTML/CSS/JavaScript through WebView2. Compared with Wails this avoids an additional desktop framework and build toolchain; compared with classic Windows widgets it permits the requested restrained visual design. WebView2 is the only system runtime prerequisite. A single portable executable contains assets and frontend.

Boundaries: `internal/history` owns raw identity, previews, encryption and persistence; `internal/clipboard` owns Win32 capture/restore; `internal/desktop` owns lifecycle, tray, positioning and frontend bindings; `internal/ui` embeds the panel; `cmd/kol` starts the app.

## Visual spec
Generated reference: `docs/design/panel-concept.png`; standalone logo: `assets/logo.png`. Full compact panel with true white surface, dark ink green text, mint selected rows, Segoe UI Variable typography, 18px panel radius, 16px main gutter, 8px spacing scale. Content typography 13px body/12px metadata; wordmark 27px. Outline icons at 18px with 1.7px strokes. Header, search field, five filter buttons, scrollable open list, quiet capture-status footer and keyboard hints. Settings use the same visual language. Empty/loading/error states are real; no seeded content in production. Logo is the separately generated production mark (intentional difference from reference's generic outline mark).

## Validation
Cross-platform unit tests for identity, whitespace, file metadata, persistence, encryption, deletion and pinning. Real Windows clipboard integration tests for round trips and custom registered formats. Linux and Windows build/tests, UI interaction checks in Playwright (Browser plugin unavailable), visual comparison with generated concept, Windows executable smoke test, and clean public-source audit before GitHub push.

## User revision, 2026-10-28
The live review asks for a more practical daily list: permanent trash beside pin controls, stable timestamp on hover, no visible information icon, smaller text rows with only the copied text, no size/copy-count/type subline, a large image preview below its label, and Ctrl+F for search. Information remains available via right-click. Details of partially captured formats belong there; only an entirely uncapturable copy or an operational failure warrants a notice in the daily panel. This revision supersedes the corresponding generated-concept details.

Implementation floor is Go 1.26, required by the selected current dependencies. Native APIs pin the calling OS thread for the full clipboard lock; the listener suppresses its own writes using the owner captured while locked and retries transient contention without requiring another change notification.
