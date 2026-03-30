# logviewer — Architecture Specification

Reference document. Read this before implementing any screen, component, or
package. See `CLAUDE.md` for always-loaded rules, API reminders, and the
mandatory key/help component pattern.

Each section below links to a dedicated doc file. Read the relevant file before
implementing that area.

---

| Section | Doc |
|---------|-----|
| 1. Data Models | [docs/data-models.md](docs/data-models.md) |
| 2. SSH Config Parsing and Host Validation | [docs/ssh-config.md](docs/ssh-config.md) |
| 3. SSH Client | [docs/ssh-client.md](docs/ssh-client.md) |
| 4. Tail Streaming | [docs/tail-streaming.md](docs/tail-streaming.md) |
| 5–6. Key and Help Components (pattern + all KeyMap definitions) | [docs/key-help.md](docs/key-help.md) |
| 7–11. Application Screens (AppModel, ProjectList, Creator, FileList, Grid) | [docs/screens.md](docs/screens.md) |
| 12. ServerCard Component | [docs/server-card.md](docs/server-card.md) |
| 13–14. Filtering and Search Mode | [docs/filtering.md](docs/filtering.md) |
| 15. Phase 2 — Log Parsing + DetailOverlay | [docs/log-parsing.md](docs/log-parsing.md) |
| 16. Styles Reference | [docs/styles.md](docs/styles.md) |
| 17. Error Handling | [docs/error-handling.md](docs/error-handling.md) |
| 18. Dependencies (go.mod) | [docs/dependencies.md](docs/dependencies.md) |
