# Client connection names and SNI discovery

Each client can have a **Connection name**, configured in the client editor. This
single name overrides protocol-specific and host-specific names in share links,
QR links, Xray JSON, sing-box/Hiddify, and Clash/Mihomo subscriptions. It also
applies to external links and external subscriptions attached to the client.

An empty name restores the existing subscription naming template. Changing this
field does not change the client email, subscription ID, or credentials. Clients
must refresh their subscription to receive the updated names.

Clash/Mihomo and Hiddify sing-box require unique proxy names. When several entries
share the configured name, they keep the common base and receive numeric suffixes
such as `My VPN`, `My VPN-2`, and `My VPN-3`. Telegram proxy deep links do not carry
a connection-name field.

The client API accepts `connectionName` as an optional string of up to 256
characters without control characters. Omitting the field preserves the stored
override; sending an empty string clears it. Export/import and node synchronization
carry the override with the client.

The SNI scanner ships with 60 candidate TLS endpoints. Existing installations
using the previous ten default candidates receive the expanded list automatically.
Custom candidate lists remain authoritative; an empty list uses the defaults.
Candidates are probes, not guaranteed compatible targets: search checks their
certificates and TLS capabilities. Reachability can differ between the panel,
remote nodes, and client networks. Ordinary TLS inbounds still need an SNI covered
by their own server certificate; selecting a remote name does not install it.
