# Talk_cut agent notes

## Go Tooling and Authorization

Follow the Go [GUIDELINES](~/prog/standards/go/GUIDELINES.md) and
[RATIONALE](~/prog/standards/go/RATIONALE.md). Read `tools/settings.rb` for this
project's artifact layout, checks, version mode, and declared installation targets.

Use the exact command forms when applicable:

| Command | Use |
|---|---|
| `rtk proxy tools/build` | Repository-local build |
| `rtk proxy tools/gate` or `rtk proxy tools/check` | Fast gate; add `--full` for release verification |
| `rtk proxy tools/lint` | Static feedback |
| `rtk proxy tools/snapshot [message]` | Local checkpoint without gate/push |
| `rtk proxy tools/commit [--full] <message>` | Requested gated local commit |
| `rtk proxy tools/bump` | Requested version bump and verified push |
| `rtk proxy tools/bump --resume` | Resume an inspected failed push without bumping again |
| `rtk proxy tools/install` | Requested installation to declared targets |
| `rtk proxy tools/release [--install] <message>` | Requested release, with installation only when authorized |

Run routine build, lint, test, and gate checks for authorized work. Existing
command approval allows execution; it does not authorize unrelated commits,
releases, installations, or changes outside the task. Reuse authorization already
given in the session. Keep sandboxing and manual approval review enabled; these
scripts do not change approval settings or writable roots. Bootstrap analysis
tools separately when authorized. Never enlarge a baseline to hide a new finding.
