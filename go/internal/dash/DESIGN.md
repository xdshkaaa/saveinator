# DESIGN.md — SAVEINATOR · DASH

Built world record for the operator dashboard at dash-saveinator.xdshka.party.
Written from the shipped implementation after the «Полевой штаб» redesign
(direction contract in `go/internal/dash/static/index.html`; seed `3a869e8c`).

## Thesis

An operator's field console, not a card dashboard. Every service, metric, and
block is an instrument on a panel: status reads as a lamp, trend as a delta,
data as a strip-chart and ledger. The category default (hero header, uniform
cards, big-number hero metrics) is refused in favor of a glance-first
instrument board: green lamps mean "в строю", the orange lamp means alarm,
deltas show direction, the table is the work.

## Own-world

- **Material:** matte dark metal. Three panel depths (`--panel #161d24`,
  `--panel-2 #1b242c`, `--panel-3 #202a34`) on a near-black ground
  (`--bg #0d1217`) with a faint cool radial glow at the top-left. Plates get
  a 1px border (`--border #242f3a`), an inset top highlight, and a soft
  drop shadow (offset + blur, never a hard offset shadow).
- **Signals:** status lamps glow (`ok #34d399` / `bad #f87171`, down lamps
  blink under reduced-motion off), the alarm accent is orange
  (`alarm #e8482c`) and reserved for failure (KPI lamp rail, brand mark,
  favicon). The UTC readout is an amber (`amber #f5b942`) tab.
  `accent #2aabee` is for links, chart total, active selection, and the
  current bot name — never decoration.
- **Type:** Manrope (UI, 500–800) + JetBrains Mono (data, labels, UTC).
  Panel titles are mono 11px uppercase with wide tracking; KPI values are
  mono 27px bold with tabular numerals; deltas and hints are mono 11px.
  Fixed rem scale, no fluid type.
- **Composition:** six KPI instruments in a row (3×2 below 1100px, 2×3 below
  640px), each with a 2px lamp rail on top that turns orange on alarm. The
  services strip is a grid of lamp tiles. The 14-day timeline is a canvas
  strip-chart (total area + completed + failed polylines) spanning the full
  panel width. Platforms are bar rows, bots are compact plates, users are a
  dense ledger table with mono numerals.
- **Grid discipline:** the shell is a single-column stack (max 1320px);
  `min-width: 0` on grid items prevents table blowout on mobile. No nested
  cards, no fractional offsets in the table.

## Surfaces

- **Login gate:** a single centered plate with the brand mark (orange ring),
  the Telegram Login Widget for @saveinator_bot, and the "Вход только для
  владельца" line. Error copy distinguishes "не в списке операторов" (403)
  from network failures.
- **Header plate:** brand, amber UTC clock, "обновлено …" mono readout,
  Обновить (primary), Выйти (ghost, red on hover, only when authed).
- **Services:** name + lamp + mono latency; hint "все в строю" or "N из M в
  строю".
- **KPI instruments:** label (mono caps), value (mono bold), delta line
  (up/down colored). Alarm state lights the top rail orange.
- **Timeline:** canvas, 4 gridlines, area under total, three polylines,
  mono axis labels; redrawn on resize and every 30 s.
- **Platforms:** name, bar (accent; warn→bad gradient when success < 70%),
  count, sub-line "успешность … · юзеров …".
- **Bots:** name (accent mono), users / downloads / 30-day downloads,
  failure count in red when > 0.
- **Users ledger:** ID (mono faint), handle + name, language chip, bot chip,
  registration, downloads, errors (red), activity; search + sort; row click
  opens the download-history drawer (status rail 2px on the left edge of
  each item; badges, meta line, error text).
- **Drawer:** right-side panel with backdrop blur, Escape/backdrop/close
  affordances, `prefers-reduced-motion` disables the slide.

## Behavior

- 30 s auto-refresh of overview/services/platforms and the timeline;
  manual refresh re-pulls users too.
- Auth: Telegram Login Widget callback → HMAC-SHA256 verification → session
  cookie (HttpOnly, Secure, SameSite=Lax, 7 days) → `/api/auth/status`
  gates the app; `/api/auth/logout` revokes. Data endpoints 401 without a
  session; the frontend returns to the gate on 401.
- The app shell is served unauthenticated; only `/api/*` data requires a
  session.

## Tokens

| Token | Value | Use |
|---|---|---|
| `--bg` | `#0d1217` | page ground |
| `--panel` | `#161d24` | plates |
| `--panel-2` | `#1b242c` | tiles, inputs, rows |
| `--panel-3` | `#202a34` | hover |
| `--border` | `#242f3a` | hairlines |
| `--border-2` | `#2c3946` | stronger hairlines |
| `--text` | `#d6dee6` | body |
| `--muted` | `#94a2b0` | secondary text |
| `--faint` | `#7d8b99` | hints, meta (≥4.5:1 on bg/panel) |
| `--accent` | `#2aabee` | links, chart, selection |
| `--ok` | `#34d399` | up states |
| `--bad` | `#f87171` | down/failure states |
| `--alarm` | `#e8482c` | alarm rail, brand mark |
| `--amber` | `#f5b942` | UTC tab, warn fill |
| `--mono` | JetBrains Mono | data, labels |
| `--sans` | Manrope | UI text |

Contrast: body text 11.5:1, muted 7.2:1, faint 5.4:1 (on bg), faint 4.9:1
(on panel). Focus rings: 2px accent with 2px offset everywhere.

## Motion

One authored moment per state: the down-lamp blink (disabled under
`prefers-reduced-motion`), the drawer slide (0.22 s ease), and the KPI value
flash on change. Everything else is instant state, in line with Operate-mode
constraints.
