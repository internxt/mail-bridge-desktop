# IMAP development connector

`NewDevelopmentConnectorFactory` creates an in-memory mailbox for testing the
local IMAP service without a backend connection.

```go
server, err := imapserver.Start(ctx, imapserver.UnlockedSession{
    AccountID: "development-account",
    Addresses: []string{"user@example.test"},
}, imapserver.Config{
    ListenAddress: "127.0.0.1:1143",
    DataDir:       ".bridge-data",
    ConnectorFactory: imapserver.NewDevelopmentConnectorFactory([][]byte{
        []byte("From: sender@example.test\r\n" +
            "To: user@example.test\r\n" +
            "Subject: Test message\r\n\r\n" +
            "Hello from the development mailbox.\r\n"),
    }),
})
if err != nil {
    return err
}
defer server.Close(context.Background())
```

Each byte slice is one complete RFC 822/MIME email message. The connector
creates an Inbox and makes those messages available to an IMAP client.

`server.Status()` returns the IMAP address and generated local credentials.
Use those values to configure a desktop email client for local testing.

This connector stores mail only in memory. Do not use it for real accounts;
the production connector will obtain mailbox data from the backend instead.
