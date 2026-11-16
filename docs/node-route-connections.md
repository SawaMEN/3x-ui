# Managed node connections

Quick routes prepare a dedicated transport identity on the destination inbound.
These identities belong to routing, not to the ordinary client lifecycle.

- The reserved `node-route-…@internal` namespace is excluded from client lists,
  paging, counts, online counts, exports, orphan removal and group aggregates.
- Existing transport UUIDs/passwords and inbound memberships remain in place;
  upgrading does not rotate credentials or interrupt saved cascades.
- Ordinary create/import/update/attach and bulk client actions reject this
  namespace. Only the internal routing provisioner can create its credentials.
- The routing page lists prepared node connections separately. A connection
  can be deleted there after removing its route references and saving settings.
- Deletion checks references in all core templates, including disabled/nested
  rules, native final destinations and infrastructure outbound selections.
  A used connection is rejected instead of silently changing traffic routing.
- Removing a connection deletes only its transport identity and memberships.
  The destination node, destination inbound and ordinary clients remain intact.
  Offline or failed remote activation is reported as pending synchronization.

The existing canonical credential storage is retained for compatibility with
node synchronization and protocol engines. Public client APIs separate the
reserved infrastructure namespace rather than requiring a destructive schema
migration or borrowing a user's credentials.
