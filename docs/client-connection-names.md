# Client connection names and SNI discovery

The client editor offers three connection naming modes:

| Mode                     | Inbound remark `Germany`, client email `ivan` |
| ------------------------ | --------------------------------------------- |
| Connection name (remark) | `Germany`                                     |
| Name-client              | `Germany-ivan`                                |
| Client                   | `ivan`                                        |

The name comes from the inbound connection's remark. Client means its email
identifier. A blank or whitespace-only remark falls back to the client email in
both remark modes, without a leading hyphen. The same rule applies to every
protocol, share/QR link, Xray JSON, sing-box/Hiddify, and Clash/Mihomo subscription.
For attached external links, the connection name is the external link's remark
or existing label.

Clash/Mihomo and Hiddify sing-box require unique proxy names. Duplicate names
receive numeric suffixes such as `Germany`, `Germany-2`, and `Germany-3`. Telegram
proxy deep links do not carry a connection-name field. Clients must refresh their
subscription to see changes. Client identity, credentials, and subscription IDs
are unaffected.

The client API accepts optional `connectionNameMode`: `remark`, `remark-client`,
or `client`. Omission preserves the saved choice. Existing clients without a mode
retain their earlier subscription naming template until a mode is saved; the
editor defaults to the remark option. Export/import and node synchronization
carry the selected mode with the client.

The SNI scanner ships with 60 candidate TLS endpoints. Existing installations
using the previous ten default candidates receive the expanded list automatically.
Custom candidate lists remain authoritative; an empty list uses the defaults.
Candidates are probes, not guaranteed compatible targets: search checks their
certificates and TLS capabilities. Reachability can differ between the panel,
remote nodes, and client networks. Ordinary TLS inbounds still need an SNI covered
by their own server certificate; selecting a remote name does not install it.
