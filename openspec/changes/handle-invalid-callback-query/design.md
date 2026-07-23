## Context

`go/internal/handler/bot.go` `Register()` wires callback query handlers via `th.HandleCallbackQueryCtx(handler, th.CallbackDataPrefix(...))` for 9 known prefixes. Telego's `telegohandler.HandlerGroup.processUpdate` checks handlers in registration order and dispatches to the first whose predicates all pass; if none match, the update is dropped with no response (confirmed by reading `telegohandler` v0.32.0 source — no built-in "default"/"not found" handler exists). Every other callback handler in this file follows the pattern `answerCallbackQuery` first (or on the terminal path) then act — so callers already expect the toast/spinner to always resolve.

## Goals / Non-Goals

**Goals:**
- Every callback query the bot receives gets an `answerCallbackQuery` response, even when its data matches no known handler.
- Unrecognized callback data is logged with enough context (query ID, user ID, chat ID if present, raw data) to investigate later without needing to reproduce.
- The user sees a small, localized error toast instead of an infinite spinner.

**Non-Goals:**
- Not validating payload *contents* within existing handlers (e.g. malformed `quality:` suffix) — that is a separate, per-handler concern the user explicitly scoped out in favor of the global fallback.
- Not adding retry/recovery logic — an invalid callback is terminal; the bot just acknowledges it.

## Decisions

- **Fallback handler registered last, no predicate**: Add `h.HandleCallbackQueryCtx(b.onUnknownCallback(bot))` (matching `th.AnyCallbackQuery()` or simply no predicate — telego allows zero predicates) as the final call in `Register()`, after all 9 prefix handlers. Since dispatch stops at first match and predicates run in registration order, this only fires when nothing else matched. Alternative considered: a middleware that checks "was this update handled" — rejected, telego middlewares run before dispatch, not after, so they can't observe whether a later handler matched.
- **Toast via `AnswerCallbackQuery` with `ShowAlert`/text, not silent ack**: Reuses the existing `tu.CallbackQuery(query.ID)` builder used elsewhere in this file (e.g. line 110, 138), with `.WithText(...)` and a locale string, so behavior is consistent with how other handlers report soft errors.
- **Locale key `callback.invalid`**: Follows existing dotted-namespace convention (`onboarding.welcome`, etc.); short user-facing message ("This action is no longer available." / RU equivalent), not for debugging.
- **Log at `slog.Warn`, not `Error`**: This is expected to happen occasionally (stale UI after restart, old messages) rather than indicate a bug — matches the existing severity choice for other recoverable/user-driven paths in this file (`slog.Warn("user lookup failed", ...)`).

## Risks / Trade-offs

- [Risk] Registering the fallback with zero predicates could accidentally shadow a future prefix handler if someone forgets it must come last → Mitigate with a comment at the fallback registration line stating it must stay last in `Register()`.
- [Risk] `query.From.ID` or `query.Message` could be nil for some malformed queries, same guard pattern already used in `onLanguageChosen` → Mirror that nil-check before logging/using those fields.

## Migration Plan

Additive only — no schema, queue, or config changes. Deploy via normal `scripts/deploy.sh`; no rollback concerns beyond reverting the commit.
