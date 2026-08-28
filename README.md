# Mail Bridge desktop

This project provides local IMAP and SMTP bridge foundations. IMAP uses
[Gluon](https://github.com/ProtonMail/gluon); SMTP currently accepts local
submissions but does not yet deliver them through the backend.

## Environment

Copy `.env.example` to `.env` to override local development settings. The
shared `BRIDGE_HOST` defaults to `127.0.0.1` and is used for local protocol
listeners; `BRIDGE_IMAP_PORT` and `BRIDGE_SMTP_PORT` set their ports. Keep the
host loopback-only. Process environment variables override values in `.env`.
Calls to `imapserver.Start` that omit `ListenAddress` use `BRIDGE_HOST` and
`BRIDGE_IMAP_PORT` as well.

## Run the development IMAP server

Install Go 1.27, then run:

```powershell
$env:CGO_ENABLED = "1"
$env:CC = '"C:\Program Files\Microsoft Visual Studio\18\Community\VC\Tools\Llvm\x64\bin\clang.exe"'
go run ./cmd/bridge
```

Gluon uses SQLite through CGO. The compiler path above is available with the
Visual Studio C++/LLVM tools used by this development environment. On another
machine, point `CC` at an installed C compiler instead.

The command starts both local IMAP and SMTP. It prints the IMAP address and a
generated password. IMAP serves one fixture message from an in-memory
development mailbox; SMTP accepts development submissions but does not yet
deliver them through the backend. Press `Ctrl+C` to stop both services.

The development command has no TLS so it is only suitable for local protocol
testing. The `imapserver.Config` API accepts a TLS configuration and enables
IMAP STARTTLS when one is supplied.

## Authentication and encrypted mail

The IMAP process never accepts the user's main account password. The desktop
application must first authenticate and unlock the account's encryption keys,
then pass an `UnlockedSession` to `imapserver.Start`. A future production
Gluon connector will fetch encrypted data from the backend, decrypt it locally
into MIME messages, and apply local IMAP changes back to the backend API.

