---
title: Styles Reference
description: All lipgloss style variables defined in ui/styles/styles.go — colours, borders, badges, and highlight styles.
---

# Styles Reference (ui/styles/styles.go)

All lipgloss styles live here. Never define styles inline elsewhere.

| Variable           | Value      | Use                               |
|--------------------|------------|-----------------------------------|
| `ColorPrimary`     | `#7B61FF`  | focused borders, key labels       |
| `ColorSuccess`     | `#2ECC71`  | connected badge                   |
| `ColorWarning`     | `#F39C12`  | connecting, paused                |
| `ColorError`       | `#E74C3C`  | disconnected, errors              |
| `ColorMuted`       | `#666666`  | unfocused borders                 |
| `ColorHighlight`   | `#F1C40F`  | filter match background           |
| `ColorMarker`      | `#3498DB`  | marker rule                       |
| `CardBorder`       | —          | Rounded border, `ColorMuted`      |
| `CardBorderFocused`| —          | Rounded border, `ColorPrimary`    |
| `MatchHighlight`   | —          | `ColorHighlight` bg, black fg     |
| `MarkerLine`       | —          | `ColorMarker`, bold               |
| `BadgeConnected`   | `●`        | green                             |
| `BadgeConnecting`  | `◌`        | yellow                            |
| `BadgeDisconnected`| `✗`        | red                               |
| `BadgePaused`      | `[PAUSED]` | yellow bold                       |

The `help.Model` uses its own default styles which fit naturally. Override
`help.Styles` if needed to match `ColorPrimary`/`ColorMuted`.
