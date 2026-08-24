<p align="center">
  <img src="docs/img/banner.svg" alt="kekkai — secure password vault" width="720"/>
</p>

<h1 align="center">kekkai 🦊</h1>

<p align="center">
  <strong>A minimal, keyboard-first password manager that lives in your terminal.</strong><br/>
  Argon2id · AES-256-GCM · interactive TUI · TOTP · zero external CLI frameworks
</p>

<p align="center">
  <a href="#-features"><img src="https://img.shields.io/badge/features-TOTP%20%7C%20HIBP%20%7C%20themes-orange" alt="features"/></a>
  <img src="https://img.shields.io/badge/crypto-Argon2id%20%2B%20AES--256--GCM-blue" alt="crypto"/>
  <img src="https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey" alt="platform"/>
  <img src="https://img.shields.io/badge/go-1.22%2B-00ADD8?logo=go&logoColor=white" alt="go"/>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green" alt="license"/></a>
</p>

<p align="center"><a href="README.ru.md">🇷🇺 Русская версия</a></p>

---

## ✨ Features

- **Interactive TUI** — vault list, forms, fuzzy search, mouse-wheel scrolling, 7 color themes
- **Strong by default** — Argon2id key derivation (t=3, m=64 MiB, p=4) + AES-256-GCM authenticated encryption
- **TOTP built in** — store a secret per entry, generate and copy 2FA codes with one keypress
- **Breach checks** — verify passwords against Have I Been Pwned via k-anonymity (the password never leaves your machine)
- **Password generator** — `Ctrl+G` creates an 18-char strong password inline
- **Clipboard hygiene** — copied secrets auto-clear after 15/35/60 seconds (configurable)
- **Idle auto-lock** — the vault locks itself after inactivity and wipes entries from memory
- **Layout-proof hotkeys** — shortcuts work identically in English and Russian keyboard layouts (`s` = `ы`, `a` = `ф`…)
- **Safe storage** — atomic writes (temp file + rename, mode 0600), secrets are wiped from memory when no longer needed
- **Portability** — single binary, no daemon, no cloud, vault is one encrypted file

## 📸 Screenshots

| Unlock | Vault |
| --- | --- |
| ![Unlock screen](docs/img/unlock.svg) | ![Vault list](docs/img/vault.svg) |

| Add entry | Settings & themes |
| --- | --- |
| ![Add entry](docs/img/add.svg) | ![Settings](docs/img/settings.svg) |

## 🚀 Installation

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/Dtierofficial/kekkai-cli/main/scripts/install.ps1 | iex
```

**Linux / macOS:**

```sh
curl -fsSL https://raw.githubusercontent.com/Dtierofficial/kekkai-cli/main/scripts/install.sh | sh
```

**From source** (Go 1.22+):

```sh
go install github.com/Dtierofficial/kekkai-cli/cmd/kekkai@latest
```

> Windows: add `%USERPROFILE%\go\bin` to `PATH`. Linux/macOS: add `$HOME/go/bin`.

## ⌨️ Quick start

```text
kekkai init     # create a vault (asks for a master password)
kekkai          # open the interactive TUI
```

Classic CLI is also available:

```text
kekkai add github.com alice
kekkai get github.com
kekkai list
kekkai delete github.com
```

## 🖥️ Keybindings

### Vault list

| Key | Action |
| --- | --- |
| `↑` / `↓` | navigate entries (mouse wheel works too) |
| `Enter` | reveal / hide password |
| `c` | copy password to clipboard |
| `u` | copy username |
| `t` | generate & copy TOTP code |
| `a` | add entry |
| `e` | edit entry |
| `d` | delete entry |
| `/` | fuzzy search / filter |
| `s` | settings |
| `Esc` | clear filter → quit |
| `q` / `Ctrl+C` | quit |

Hotkeys are **layout-independent**: in Russian layout press the same physical keys (`ы` acts as `s`, `ф` as `a`, …). Works with Shift and CapsLock too.

### Add / edit form

| Key | Action |
| --- | --- |
| `Tab` / `Enter` | next field |
| `Backspace` | delete character |
| `Ctrl+G` | generate a strong password into the field |
| `Ctrl+H` | check the password against HIBP |
| `Esc` | cancel |

### Confirmations & login

| Key | Action |
| --- | --- |
| `y` / `Enter` | confirm deletion / export |
| `n` / `Esc` | cancel |
| `Enter` (login) | unlock vault |
| `Esc` (login) | quit |

## ⚙️ Settings

Open with `s`. Navigate with `↑`/`↓`, change with `Enter`:

| Setting | Values |
| --- | --- |
| Lock timeout | Off · 1m · 5m · 15m (default 5m) |
| Clipboard clear | 15s · 35s · 60s |
| New master | type a new master password, `Enter` to apply |
| Theme | Fox Orange · Cyberpunk Neon · Dracula · Matrix Green · Deep Purple · Nordic Blue · Monochrome |
| Import from CSV | merge entries from a CSV: kekkai's own exports (`service`) or Bitwarden/Chrome-style files (`url`/`name`, `username`, `password`, optional `totp`) |
| Export Backup (.enc) | encrypted portable backup |
| Export to CSV (Unsafe) | plaintext export — use with care |

## 🔐 Security model

- Master password → **Argon2id** (time=3, memory=64 MiB, threads=4) → 256-bit key
- Vault body encrypted with **AES-256-GCM** (12-byte nonce, random per save)
- File layout: `KKV1` magic + version + salt + nonce + ciphertext; format versioning supported
- Writes are **atomic**: temp file (mode 0600) + rename — a crash can't corrupt the vault
- Plaintext secrets are **zeroed in memory** as soon as they're no longer needed
- No telemetry, no network access except the optional HIBP check (k-anonymity: only a 5-char SHA-1 prefix is sent)

⚠️ **There is no recovery.** Lose the master password — lose the vault. The file is unreadable without it.

## 📁 File locations

| What | Windows | Linux / macOS |
| --- | --- | --- |
| Vault | `%AppData%\kekkai\vault.kek` | `$XDG_CONFIG_HOME/kekkai/vault.kek` or `~/.config/kekkai/vault.kek` |
| Config (theme) | `%AppData%\kekkai\config.json` | same pattern |

## 🛠️ Building from source

```sh
git clone https://github.com/Dtierofficial/kekkai-cli.git
cd kekkai-cli
go build -o kekkai ./cmd/kekkai
go test ./...
```

```
kekkai-cli/
├── cmd/kekkai/    # TUI, CLI commands, console setup
├── vault/         # encryption, storage format, atomic I/O
└── scripts/       # install.ps1 / install.sh
```

---

<p align="center">Made with Go and a fox. Keep your keys in the den. 🦊</p>
