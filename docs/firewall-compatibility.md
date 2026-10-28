# Firewall backend compatibility

The panel probes the real INPUT ruleset before selecting a native iptables
backend. A successful `iptables --version` alone does not prove kernel support.
If nf_tables reports `Could not fetch rule set generation id: Invalid argument`
or a backend-not-supported error, the panel can use installed `iptables-legacy`
and `ip6tables-legacy` together. It does not change system alternatives.
Permission errors and conflicting nftables input hooks are not bypassed.

If UFW is already enabled in `/etc/ufw/ufw.conf` and cannot read its ruleset,
the panel reports the compatibility problem instead of installing a second
filtering layer over it. Repair the host's UFW backend first. Depending on the
operating system, that requires updating the host kernel or selecting matching
legacy alternatives for both iptables and ip6tables, then reloading UFW. Keep
provider console access while making host firewall changes. Containers may
require a kernel change on their host; a panel update cannot supply missing
kernel functionality.

Automatic reconciliation starts when the authenticated firewall page is opened,
including on older installations without an initialization marker. It preserves
an explicitly disabled firewall and a disabled automatic-sync preference. The
background worker also runs without an open browser. The page refreshes status
every 15 seconds while visible; the manual sync button is no longer required.
