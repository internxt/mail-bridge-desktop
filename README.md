# Mail Bridge desktop

A local IMAP and SMTP server that lets an ordinary mail client read Internxt
mail. Messages are encrypted on the server, so the bridge fetches them, decrypts
them on the machine and serves them as ordinary MIME. IMAP uses
[Gluon](https://github.com/ProtonMail/gluon).

## Running it in development

The bridge does not start on its own. It expects a parent — the desktop app — to
connect over a control channel and hand it the account's session: the API token,
the encryption key, and the password a mail client will use. `cmd/devcontrol`
stands in for that parent, so two terminals are needed:

```
make dev-control    # the stand-in parent, waits for the bridge
make run            # the bridge itself
```

`dev-control` prints the settings to put in a mail client once the bridge is up.
The password it generates is kept in `.bridge-data/`, so it survives restarts and
the client does not have to be reconfigured.

Before the first run, copy `.env.example` to `.env` and fill in the account:

| Variable                            | What it is                                                                                                                                                                                                                        |
| ----------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `BRIDGE_DEV_EMAIL`                  | The address to serve                                                                                                                                                                                                              |
| `BRIDGE_DEV_TOKEN`                  | Mail API token                                                                                                                                                                                                                    |
| `BRIDGE_DEV_ENCRYPTION_PRIVATE_KEY` | The account's mail key, base64 of 32 bytes                                                                                                                                                                                        |
| `MAIL_API_URL`                      | Where the Mail API runs                                                                                                                                                                                                           |
| `MAIL_SERVER_PUBLIC_KEY`            | The Mail API's own public key, base64. Seals mail sent to a recipient without an Internxt key of their own, so it reaches the backend encrypted and never travels in the clear to whatever external provider serves that address. |

Without the key the bridge still runs: it lists mail and serves what it cannot
decrypt as it arrived, which is also what happens for a message encrypted for
another address.

`make help` lists every target.

### When mail looks wrong

Gluon stores the message it is given during the initial sync and serves that copy
from then on. **Any change to how a message is built only shows up after clearing
its cache:**

```
rm -rf .bridge-data/data .bridge-data/database
```

Stop the bridge first, or it writes the cache back on exit. Leave
`.bridge-data/dev-mail-password` alone unless you want to reconfigure the client.

To see what a client is actually asking for, set `BRIDGE_LOG_IMAP_PROTOCOL=true`
and the IMAP conversation goes to the log, each line marked with the side it came
from. It carries subjects and bodies, so it is for debugging rather than for a
running bridge.

## Connecting a mail client

IMAP on `127.0.0.1:1143` and SMTP on `127.0.0.1:2025`, with the username and
password `dev-control` printed, and **no encryption** — the connection never
leaves the machine, but a client that insists on TLS will refuse it.

## Environment

Copy `.env.example` to `.env` to override local settings. `BRIDGE_HOST` defaults
to `127.0.0.1` and `BRIDGE_IMAP_PORT` / `BRIDGE_SMTP_PORT` set the ports. Keep
the host loopback-only. Process environment variables override values in `.env`.

## Authentication and encrypted mail

The bridge never sees the user's main account password. The desktop app
authenticates, unlocks the account's keys and sends them over the control
channel, where they stay in memory for as long as the process runs: nothing about
the account is written to disk, so signing out is a matter of stopping the
bridge.

The one exception is the passphrase encrypting Gluon's cache, which the parent
does not send. It is stored, because regenerating it would leave the cache
unreadable and resynchronise every mailbox on each start.

[`internal/crypto`](internal/crypto/README.md) documents how the encryption for mail works.

## Using it on the desktop apps

The mail bridge is consumed by the Internxt desktop apps on [Windows](https://github.com/internxt/drive-desktop), [Linux](https://github.com/internxt/drive-desktop-linux), and [macOS](https://github.com/internxt/drive-desktop-macos). It shouldnt be be compiled by those apps.
A mail-bridge release produces immutable binaries so each desktop app fetches one pinned binary at packaging time and includes it in its own application bundle.

## Release artifact contract

Every bridge Git tag in the form `vMAJOR.MINOR.PATCH` publishes a GitHub
Release (or an equivalent immutable artifact store) with the following assets.
`VERSION` below is the tag without its leading `v`.

| Target              | Archive                                   | Executable inside archive |
| ------------------- | ----------------------------------------- | ------------------------- |
| Windows x64         | `mail-bridge_VERSION_windows_amd64.zip`   | `mail-bridge.exe`         |
| Linux x64           | `mail-bridge_VERSION_linux_amd64.tar.gz`  | `mail-bridge`             |
| macOS Apple Silicon | `mail-bridge_VERSION_darwin_arm64.tar.gz` | `mail-bridge`             |

Each archive contains exactly one executable at the archive root. It must not
contain a directory named after the version or target. This makes the path
inside an Electron resource bundle stable.

The release also contains:

- `manifest.json`, conforming to
  [`release/manifest.schema.json`](release/manifest.schema.json).
- `checksums.txt`, with one line per binary archive in standard SHA-256 format:
  `<lowercase-hex-sha256>  <filename>`.

The manifest is the consumption contract. Consumers select an entry by `os`
and `arch`, download `url`, verify `sha256` against the downloaded archive,
and extract the named `executable`. They must pin an exact `version` so they
must never request a moving `latest` release.
