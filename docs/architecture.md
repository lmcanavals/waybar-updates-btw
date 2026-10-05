# Project Documentation: Waybar Updates Btw

This document provides a comprehensive technical specification of the `waybar-updates-btw` project, detailing its architecture, internal design patterns, D-Bus communication protocol, formatting algorithms, and configuration parameters.

---

## 1. Architectural Overview

The project employs a split **Client-Server architecture** using the **D-Bus Session Bus** as communication middleware. This design resolves critical issues common in status-bar modules:

1. **Performance & Latency:** Pacman mirror checks and HTTP requests to the Arch User Repository (AUR) are deferred to a background daemon, preventing status-bar lag or freezes.
2. **Rate Limiting & Bandwidth:** The client only queries when the daemon notifies it of changes, reducing unnecessary requests.
3. **Database Locks:** Utilizes the safe `checkupdates` script under the hood, which safely avoids pacman database locks by operating on an isolated temporary database copy.

```mermaid
graph TD
    subgraph System
        PacmanDB[(Pacman Repos)]
        AurAPI[AUR RPC API v5]
    end

    subgraph Service Layer (Background Daemon: updates-fetch)
        Fetch["cmd/updates-fetch (Entrypoint)"]
        Server["internal/server (Daemon State & Timers)"]
        CheckPacman["internal/pacman (checkupdates Runner)"]
        CheckAUR["internal/aur (AUR Client & Foreign Pkgs)"]
        ALPM["internal/alpm (vercmp / EVR Parser)"]
    end

    subgraph IPC Layer (D-Bus)
        DBusSession["D-Bus Session Bus"]
        Protocol["internal/protocol (Contracts & Schemas)"]
    end

    subgraph Client Layer (Status Bar Interface: updates-query)
        Query["cmd/updates-query (Client)"]
        Format["internal/format (Pango Markup & Version Categorizer)"]
        Waybar[Waybar / Polybar]
    end

    %% Daemon internal loops
    Fetch --> Server
    Server --> CheckPacman
    Server --> CheckAUR
    CheckAUR --> ALPM
    CheckPacman -. Runs every 1m .-> PacmanDB
    CheckAUR -. Runs every 5m .-> AurAPI
    CheckPacman -- Updates --> Server
    CheckAUR -- Updates --> Server

    %% D-Bus interaction
    Server -- "Registers Name & Exports Object" --> DBusSession
    Server -- "Emits InfoUpdated(version)" --> DBusSession
    Protocol -. Shared Types & Constants .-> Server
    Protocol -. Shared Types & Constants .-> Query
    Query -- "Listens to InfoUpdated" --> DBusSession
    Query -- "Calls GetUpdates(version)" --> DBusSession
    DBusSession -- "Returns JSON Payload" --> Query

    %% Client output
    Query --> Format
    Format -- "Outputs JSON stdout" --> Waybar
```

---

## 2. Component Specifications

### 2.1 Daemon: `updates-fetch`

The daemon [`cmd/updates-fetch`](file:///home/lmcs/Apps/repos/waybar-updates-btw/cmd/updates-fetch/main.go) is a persistent background service managed via a systemd user service.

It initializes the server engine in [`internal/server`](file:///home/lmcs/Apps/repos/waybar-updates-btw/internal/server/server.go) which:
- Claims the D-Bus bus name `org.lmcs.DBus.UpdatesBtw`.
- Exposes the object path `/org/lmcs/DBus/UpdatesBtw/GetUpdates` implementing the interface `org.lmcs.DBus.UpdatesBtw.UpdatesInterface`.
- Manages an in-memory cache of updates with thread-safe read/write locking via `sync.RWMutex`.
- Maintains independent concurrency locks (`isCheckingPacman` and `isCheckingAur`) to prevent long-running AUR API calls from blocking or dropping Pacman checks.
- Runs concurrent workers to monitor packages:

#### Pacman Checker ([`internal/pacman`](file:///home/lmcs/Apps/repos/waybar-updates-btw/internal/pacman/pacman.go))

- **Interval:** 1 minute.
- **Mechanism:**
  - Every 10 iterations (10 minutes), it performs a **Full Sync** by executing `checkupdates --nocolor`. This safely downloads fresh repository databases into a temporary database directory.
  - On the other 9 iterations, it performs a **Fast Check** using `checkupdates --nosync --change --nocolor`. Because `/tmp/checkup-db-$UID/local` is a symlink to `/var/lib/pacman/local`, this fast check instantly detects when the user performs a manual system update (`pacman -Syu`) and resets the update count without contacting mirrors.
  - Captures `stdout` and `stderr` to provide actionable diagnostic logging on failures.
  - Properly handles exit codes: exit code `2` designates that no updates are available.

#### AUR Checker ([`internal/aur`](file:///home/lmcs/Apps/repos/waybar-updates-btw/internal/aur/aur.go))

- **Interval:** 5 minutes.
- **Mechanism:**
  - Queries local "foreign" packages via `pacman -Qm`.
  - Submits batch info requests to the Arch Linux AUR RPC API (v5) at `https://aur.archlinux.org/rpc/` with multiple package names.
  - Compares the remote version with the local version using the in-memory ALPM version comparator ([`internal/alpm`](file:///home/lmcs/Apps/repos/waybar-updates-btw/internal/alpm/alpm.go)).
  - Formats results as: `aur/<pkgname> <local_version> -> <remote_version>`.

#### ALPM Version Comparison ([`internal/alpm`](file:///home/lmcs/Apps/repos/waybar-updates-btw/internal/alpm/alpm.go))

- Native Go implementation of `libalpm` / `vercmp` (`parseEVR` and `rpmvercmp`).
- Accurately splits `[epoch:]pkgver[-pkgrel]` and compares numeric segments as integers and alphabetic segments lexicographically.
- Eliminates the performance overhead of spawning external `vercmp` subprocesses for each foreign package.

---

### 2.2 Client: `updates-query`

The client [`cmd/updates-query`](file:///home/lmcs/Apps/repos/waybar-updates-btw/cmd/updates-query/main.go) is a lightweight CLI utility that:

- Establishes a connection to the D-Bus Session Bus.
- Subscribes to the `InfoUpdated` signal matching:
  `type='signal',interface='org.lmcs.DBus.UpdatesBtw.UpdatesInterface',member='InfoUpdated',path='/org/lmcs/DBus/UpdatesBtw/GetUpdates'`
- Queries the daemon on startup, on a custom time interval (default 120s), and instantly upon receiving the `InfoUpdated` signal.
- Caches the last known server version and only pulls details if the server version increments.
- Utilizes [`internal/format`](file:///home/lmcs/Apps/repos/waybar-updates-btw/internal/format/format.go) to pad columns and apply Pango markup color tags before outputting a JSON object to standard output for Waybar consumption.

---

## 3. D-Bus Interface & API Reference

### 3.1 D-Bus Configuration

| Parameter       | Value                                                                              |
| :-------------- | :--------------------------------------------------------------------------------- |
| **Bus Name**    | `org.lmcs.DBus.UpdatesBtw`                                                         |
| **Object Path** | `/org/lmcs/DBus/UpdatesBtw/GetUpdates`                                             |
| **Interfaces**  | `org.lmcs.DBus.UpdatesBtw.UpdatesInterface`, `org.freedesktop.DBus.Introspectable` |

### 3.2 D-Bus Methods

#### `CheckNow`

Immediately triggers a full check of both Pacman (`checkupdates`) and AUR packages concurrently in the background, debounces if a check is already running, resets periodic timers, and broadcasts `InfoUpdated`.

- **Signature:** `CheckNow()`
- **Input Parameters:** None
- **Return Type:** None (void)

#### `GetUpdates`

Queries the daemon for the current package updates list.

- **Signature:** `GetUpdates(clientVersion int64) (string, error)`
- **Input Parameters:**
  - `clientVersion` (`int64`): The last version number the client received. Set to `-1` for initial request.
- **Return Type:**
  - `jsonString` (`string`): A JSON-serialized representation of [`ResponseData`](file:///home/lmcs/Apps/repos/waybar-updates-btw/internal/protocol/dbus.go#L11).

##### JSON Response Schema (`ResponseData`)

```json
{
  "type": "object",
  "properties": {
    "changed": {
      "type": "boolean",
      "description": "True if the server state has a higher version than clientVersion"
    },
    "version": {
      "type": "integer",
      "description": "The current version ID of the daemon state"
    },
    "updates": {
      "type": "array",
      "items": { "type": "string" },
      "description": "List of updates (omitted if changed is false)"
    },
    "count": {
      "type": "integer",
      "description": "Total number of available updates"
    },
    "timestamp": {
      "type": "string",
      "format": "date-time",
      "description": "RFC3339 timestamp of the last state change"
    }
  },
  "required": ["changed", "version", "count", "timestamp"]
}
```

_Example Response (updates available):_

```json
{
  "changed": true,
  "version": 4,
  "updates": [
    "linux 6.9.1.arch1-1 -> 6.9.2.arch1-1",
    "aur/yay 12.3.5-1 -> 12.3.6-1"
  ],
  "count": 2,
  "timestamp": "2026-06-19T21:00:00-05:00"
}
```

_Example Response (no changes):_

```json
{
  "changed": false,
  "version": 4,
  "count": 2,
  "timestamp": "2026-06-19T21:00:00-05:00"
}
```

### 3.3 D-Bus Signals

#### `InfoUpdated`

Broadcasts that the package cache has been refreshed with new modifications.

- **Signature:** `InfoUpdated(version int64)`
- **Payload:**
  - `version` (`int64`): The new version identifier.

---

## 4. Client Output Format & Formatting Logics

The client processes update strings to produce a structured JSON object consumed by Waybar.

### 4.1 Output JSON Schema

```json
{
  "type": "object",
  "properties": {
    "text": {
      "type": "string",
      "description": "Short icon text and update count for status bar display"
    },
    "tooltip": {
      "type": "string",
      "description": "Pango markup-formatted list of package updates, separated by newlines"
    },
    "class": {
      "type": "string",
      "enum": ["updated", "has-updates"],
      "description": "CSS class hook for custom styling"
    },
    "alt": {
      "type": "string",
      "enum": ["updated", "has-updates"],
      "description": "Alternative text representation"
    }
  },
  "required": ["text", "tooltip", "class", "alt"]
}
```

#### No Updates Available

```json
{
  "text": "",
  "tooltip": "All packages are up to date",
  "class": "updated",
  "alt": "updated"
}
```

#### Updates Available

```json
{
  "text": "󰮯 2",
  "tooltip": "<span font-family='monospace' color='#ff9e64'>linux     6.9.1.arch1-1 -> 6.9.2.arch1-1</span>\n<span font-family='monospace' color='#e0af68'>aur/yay   12.3.5-1      -> 12.3.6-1</span>",
  "class": "has-updates",
  "alt": "has-updates"
}
```

### 4.2 Formatting & Version Parsing Algorithm

The client classifies updates into update severity levels to color-code them. The categorization algorithm ([`ParseVersion` in `internal/format/version.go`](file:///home/lmcs/Apps/repos/waybar-updates-btw/internal/format/version.go#L5)) performs a character-by-character comparison of the old and new version strings:

1. It iterates through characters of the version strings until it finds the first index where characters differ.
2. It counts how many separator characters (dots `.` or hyphens `-`) occurred _before_ that index of first difference.
3. The separator count determines the update category:

| Dot/Hyphen Count | Update Category               | Default Color (Hex) | CLI Flag       |
| :--------------: | :---------------------------- | :------------------ | :------------- |
|      **0**       | Major Version Update          | `f7768e`            | `-color-major` |
|      **1**       | Minor Version Update          | `ff9e64`            | `-color-minor` |
|      **2**       | Patch Version Update          | `e0af68`            | `-color-patch` |
|      **3**       | Pre-release / Revision Update | `9ece6a`            | `-color-pre`   |
|     **>= 4**     | Other Update                  | `7dcfff`            | `-color-other` |

If columns are enabled (default, `-raw-output=false`), [`AddFormat`](file:///home/lmcs/Apps/repos/waybar-updates-btw/internal/format/format.go#L17) pads package names and old version strings using computed maximum lengths among all current updates to create an aligned table layout.

---

## 5. Client Command Line Interface (CLI)

The `updates-query` client accepts the following command-line flags:

| Flag           | Type     | Default  | Description                                                               |
| :------------- | :------- | :------- | :------------------------------------------------------------------------ |
| `-check-now`   | `bool`   | `false`  | Triggers an immediate full check on the `updates-fetch` daemon and exits. |
| `-interval`    | `int`    | `120`    | Interval between active D-Bus queries in seconds.                         |
| `-raw-output`  | `bool`   | `false`  | Disables formatting the tooltip text into aligned columns.                |
| `-no-color`    | `bool`   | `false`  | Disables coloring packages by version category.                           |
| `-color-major` | `string` | `f7768e` | Hex color code for major version changes.                                 |
| `-color-minor` | `string` | `ff9e64` | Hex color code for minor version changes.                                 |
| `-color-patch` | `string` | `e0af68` | Hex color code for patch updates.                                         |
| `-color-pre`   | `string` | `9ece6a` | Hex color code for revision/pre updates.                                  |
| `-color-other` | `string` | `7dcfff` | Hex color code for other version bumps.                                   |
