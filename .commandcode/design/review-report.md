# Review Report: Saveinator Grafana Dashboards

## Summary

11 auto-provisioned Grafana dashboards for the Saveinator Telegram bot monitoring stack. The dashboards are functionally comprehensive — covering bot health, downloads, user activity, errors, infrastructure, and logs — but visually generic with no design intent. They use Grafana's defaults in every category: default theme, default colors, default typeface, no brand identity. The data coverage is strong, but the experience of using them is indistinguishable from any other monitoring setup.

**Score: 21/50**

---

## First Impression — 4/10

The first thing that hits is: there is no first thing. No dashboard makes an immediate visual claim. Opening "Saveinator Unified Monitoring" shows a row of six stat panels that all look identical — identical green-on-dark backgrounds, identical sizing, no color to distinguish criticality from routine. The title bar says "Saveinator" but the visual language says "out-of-box Grafana."

The "Logs" folder of dashboards (User Activity, Error & Reliability) leans heavily on raw Loki log panels, which display as crowded monospace text tables. These sections of the dashboard feel like a debugging terminal, not a monitoring experience. A visitor in the first 5 seconds would not know what platform this monitors, what service it supports, or why this setup is specific to Saveinator.

**What moves the score:**
- A brand color injection (one named hue across dashboard headers, stat backgrounds, panel borders)
- Thumbnail or description annotations per dashboard
- Replacing raw log panels with aggregated views where possible

---

## Hierarchy — 5/10

Panel grid layouts are logically grouped in most dashboards: stat row at top, timeseries below. The Saveinator Overview dashboard uses a clean 6-column stat row, then pairs timeseries panels in 12×8 unit blocks — a proven pattern.

However, several dashboards lose the user:

- **User Activity** interleaves log panels with timeseries across varying widths (12×8, 8×8, 8×8) in a way that creates visual noise. The first-time viewer has to parse each panel title to understand the layout pattern.
- **Error & Reliability** is the weakest — 11 panels, 7 of which are raw logs. Log panels dominate the visual field with repetitive text, making it hard to spot anomalies in the timeseries panels interspersed between them.
- No use of Grafana rows or collapsible sections to group related panels (e.g., "Download Metrics" → expandable section).
- Panel titles lack naming consistency: `"Errors / min"` vs `"Error Rate"` vs `"RPC Failures 1h"` across different dashboards for what appear to be similar metrics.
- No cross-dashboard navigation links make it harder to jump between related views (e.g., from "Downloads" to "Error & Reliability" when investigating failures).

**What moves the score:**
- Collapse dpanels into named row sections
- Create cross-dashboard links with `$__url_time_range` parameters
- Normalize title convention across all dashboards
- Reduce log panel count in favor of aggregation queries
- Standardize grid widths (favor 12×8 or 8×8 blocks, avoid interleaving)

---

## Color Voice — 2/10

Color is the weakest lens. There is no intentional color system.

- All panels use Grafana's default color sequence (blue, green, red, orange, purple) assigned by panel order.
- The platform dimension — YouTube, TikTok, Instagram, X, Pinterest, Spotify, SoundCloud — is color-coded differently in every dashboard where it appears. A YouTube line is green in one panel and blue in another.
- Stat panels use the Grafana default green/yellow/red threshold coloring, which is fine for health checks but creates a flat visual field when all six stat panels are green.
- No semantic color roles: "critical," "warning," "info" — everything looks the same until it breaks.
- No brand hue. The dashboards could belong to any service with zero visual cues otherwise.
- Hex values from dashboard JSON confirm: no custom colors defined anywhere. All colors are assigned at render time by Grafana.

**What moves the score:**
- Assign a fixed color palette to platforms and reuse it across all dashboards
- Define semantic color roles (error = red, warning = amber, normal = green, neutral = slate)
- Add a brand accent color for stat panel headers or dashboard title bars
- Use color to encode severity tier, not panel position

---

## Type Voice — 5/10

Grafana uses Inter by default, which is a competent system font for data dashboards. The problem is not the typeface but what the type says.

- Panel titles are inconsistent: `"Downloads Enqueued by Platform"`, `"Successful Downloads"`, `"Failed Downloads"`, `"Failure Rate by Platform"` — each uses a different verb and structure. Some end with "/min", some with "per Day", others omit the interval entirely.
- Legend formatting is inconsistent: `"messages/min"` vs `"active message events"` vs `"daily message events"` vs `"active chats"`. A viewer must read the query to understand what unit is displayed.
- No panel descriptions (Grafana's description/tooltip field is empty in every dashboard).
- Stat panel titles do not include units, so `"Bot Uptime"` shows seconds but the panel label doesn't say so — the unit is set in `fieldConfig.units` but not visible in the title.
- Some titles are technically accurate but user-hostile: `"HTTP Handler Latency p95"` is correct but reads as engineer shorthand.
- The "Saveinator" brand name is present in dashboard titles but not reinforced in panel naming or annotations.

**What moves the score:**
- Adopt a naming convention: `[Metric] ([interval])` — e.g., `"Download Rate (1h)"`, `"Error Rate (5m)"`
- Add panel descriptions explaining what each metric measures and when to investigate
- Include units in panel titles or subtitles where the auto-unit isn't obvious
- Standardize legend formatting across all panel targets
- Add a dashboard description explaining the dashboard's purpose at a glance

---

## Interaction Feel — 5/10

Standard Grafana interactions work as expected. Each dashboard has:
- 30-second auto-refresh
- Time range picker
- Hover tooltips on timeseries
- Legend toggle visibility

The gaps:

- **No template variables.** Not a single dashboard uses Grafana variables for filtering. Users cannot filter by platform, time window, or error type without editing the query. This is the largest missed opportunity — every dashboard that shows platform-dimensioned data would benefit from a `$platform` variable.
- **No annotations.** There are no deployment markers, restart markers, or version changes marked on the timelines. When investigating an error spike, the operator has no visual cue about whether a deploy happened at that time.
- **No linked dashboards.** From "Download Operations" you cannot click through to "Error & Reliability" to see the related errors. Every dashboard is an island.
- **No repeated panels.** All panels are individually defined rather than using Grafana's repeat feature, meaning every platform dimension is hardcoded as a separate query line.
- **Log panels are pure text dumps.** The Loki log panels (7 out of 11 in Error & Reliability, 6 out of 12 in User Activity) are raw log streams with basic label filtering. No aggregation, no pattern detection, no derived metrics. The user scrolls through log lines looking for problems — an exhausting interaction pattern.
- **The "Top Recurring Error Messages" panel (reliability-errors.json) has a query that doesn't match its title.** It groups by container, not by error message content. The panel title promises error message ranking but the query groups by container name.

**What moves the score:**
- Add template variables for platform, error source, and time range
- Add deployment annotations via Grafana API or a metrics endpoint
- Add dashboard links for related dashboards
- Refactor hardcoded queries into repeating panels
- Replace most raw log panels with derived metrics (error rate by pattern, log volume by severity)
- Fix the "Top Recurring Error Messages" query to actually group by error message pattern

---

## Smell Lens

This monitoring setup has a strong "out-of-box" smell. The tells:
- Default Grafana dark theme, no custom CSS or branding
- No custom color palette — Grafana's built-in cycler is doing all the work
- Dashboard titles are descriptive but generic ("Telegram Bots", "Download Operations") — they name the data source, not the product or the question the operator should answer
- No template variables means each dashboard is built for a single viewport with no filtering
- Heavy reliance on raw log panels (Loki) as a substitute for derived metrics — "just dump the logs" is the monitoring equivalent of a dump truck
- Interleaving log panels with timeseries panels without visual separation creates noise

The most damning smell: these dashboards could be imported into any Grafana instance watching any Telegram bot and they would look exactly the same. There is nothing Saveinator-specific about the visual presentation.

---

## Top Issues (by impact)

1. **No template variables** — The single biggest quality jump would be adding `$platform`, `$source`, `$time_range` variables. This alone triples the usefulness of every dashboard without adding panels.
2. **No color system** — Platforms need fixed colors. Severity needs semantic hues. The current default cycle makes comparison across panels harder than it needs to be.
3. **Log panel overuse** — 7 of 11 panels in Error & Reliability are raw log dumps. Replace most with aggregated error rate timeseries, pattern-matched counts, or grouped bar gauges.
4. **No cross-dashboard navigation** — Every dashboard is an island. Adding dashboard links with time range propagation is low effort, high impact.
5. **Inconsistent title/legend naming** — The user reads inconsistency as sloppiness, even when the queries are correct.
6. **No annotations** — Without deploy markers, operators can't correlate error spikes with changes.

---

## Recommendations

| Issue | Tool |
|---|---|
| No color system | `/design recolor grafana-dashboards` |
| Flat visual hierarchy, log panel overload, no navigation | `/design relayout grafana-dashboards` |
| Inconsistent titles, missing descriptions, unclear units | `/design typeset grafana-dashboards` |
| Missing variables, annotations, repeating panels, click-through | `/design interaction grafana-dashboards` |
| Pre-ship quality pass on all 11 dashboards | `/design finish grafana-dashboards` |

The dashboards are monitoring surfaces (operate register), so `surface` and `interaction` tools are the natural next modes. `recolor` should come first because color is the weakest dimension and fixing it will immediately improve perceived quality across all dashboards.
