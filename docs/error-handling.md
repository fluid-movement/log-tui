---
title: Error Handling
description: Per-scenario error behaviour table covering SSH failures, host key issues, auth failures, and disconnection during tail.
---

# Error Handling Summary

| Scenario                     | Behaviour                                                      |
|------------------------------|----------------------------------------------------------------|
| Host unreachable at FileList | Error badge on that host; others proceed                       |
| `ls` fails on log path       | Warning on that host; others show files                        |
| Auth failure                 | `FailAuthFailed`; if passphrase key: explain `ssh-add`         |
| Host key changed             | `FailHostKeyChanged`; show error + known_hosts location        |
| Host key unknown             | Modal prompt "Add to known_hosts? [y/N]"                      |
| SSH alias renamed            | Soft hostname match; warn if matched via fallback              |
| Disconnection during tail    | Badge update; exponential backoff reconnect (max 10 attempts)  |
| Unresolvable host            | Skip; warn; continue with resolved hosts                       |
