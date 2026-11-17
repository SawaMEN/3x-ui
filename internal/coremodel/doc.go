// Package coremodel contains data shared by core adapters, the panel database,
// subscriptions and node synchronization. It must not depend on a core engine.
// InboundConfig preserves the historical panel schema; it is not a native
// Hiddify configuration. Each adapter owns its wire format and validation.
package coremodel
