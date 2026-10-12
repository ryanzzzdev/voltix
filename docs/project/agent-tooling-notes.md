# Agent tooling notes — Devin edit-tool emoji corruption and write prompts

Recorded: 2026-10-12
Scope: known failure modes of the coding-agent file tools in this workspace,
with diagnosis evidence and the working mitigations. Not part of the Voltix
application; retained so future sessions do not re-debug this.

## Symptoms observed

- Repeated `edit` calls against `docs/project/backlog.md` failed with
  "Canceled due to user interrupt" across several sessions, always around
  status-emoji changes (`🚧` U+1F6A7, `🟢` U+1F7E2).
- An apparently successful `edit` left `�` (U+FFFD) on disk; `grep -c "�"`
  found 3 corrupted characters.
- `internal/device/fakeserver_test.go` appeared to revert under the compiler:
  tool reads showed new content while `go build` saw the old file (a separate
  stale-editor-buffer merge issue, resolved by the user).

## Root cause — byte-level evidence

The `edit` tool corrupts **4-byte (non-BMP) Unicode characters** on write
(the `write` tool was also implicated in earlier failures, though it emitted
correct emoji when last exercised):

| Emoji | Code point | UTF-8 | Via `edit` tool | Via Python |
|---|---|---|---|---|
| `🟢` | U+1F7E2 | `f0 9f 9f a2` | `ef bf bd` = `�` | correct |
| `🚧` | U+1F6A7 | `f0 9f 9a a7` | corrupted the same way | correct |
| `✅`, `📋` | U+2705, U+1F4CB | 3-byte (BMP) | fine | fine |

`xxd` on the affected line showed `ef bf bd` where `f0 9f 9f a2` was written.
This explains why only 🚧 and 🟢 ever failed: they are astral-plane emoji,
while every BMP status emoji had always worked.

## Contributing factor — permission prompt cascade

In Devin CLI **Normal** permission mode, every `edit`/`write`/`exec` call
prompts for approval (per `reference/permissions.mdx`):

- Batched edits queue one prompt per call.
- Declining or interrupting a prompt cancels the remaining tool calls in that
  turn — this produced the "stuck agent" appearance.
- `backlog.md` was simply the file most often receiving several edits per
  turn, so it was where the failures surfaced.

## Mitigations in place

1. **Emoji go through scripted writes.** Status/status-index changes are
   applied via `cat >>` heredocs or `python3` file writes instead of
   `edit`/`write`. These paths emit correct UTF-8 and also avoid the prompt
   queue (`Exec(cat)` is already allow-listed).
2. **Workspace write auto-approval.** `.devin/config.local.json` contains:

   ```json
   { "version": 1, "permissions": { "allow": ["Write(**)"] } }
   ```

   File edits inside the workspace no longer prompt (exec still does).
   The file is gitignored — it is machine-local, not shared project config.
3. **Session alternative:** `/mode accept-edits` (or `Shift+Tab`) grants the
   same auto-approval without a config file; `/mode smart` also covers
   routine exec/fetch calls judged safe.

## Verification

- `grep -rn "�" docs/` returns zero occurrences.
- `xxd` shows `f09f9fa2` for each `🟢` byte sequence on disk.
- All status flips after this note are applied via scripted writes; no
  corrupted diffs expected.

## Residual notes

- Non-ASCII content in general (e.g. `—`, `–`, `×`) is BMP/2-byte and writes
  correctly; only 4-byte characters were observed corrupting.
- The `ask_user_question`/`edit` tool approval prompts are per call — batch
  edits to the same file are best kept to one call per response, or scripted.
- Related records: journal entry "Diagnose recurring edit-tool stalls" in
  `docs/project/journal/2026-10.md`.
