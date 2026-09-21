package singbox

import (
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestDecodeConnectionMapping(t *testing.T) {
	var data []byte
	data = protowire.AppendTag(data, 1, protowire.BytesType)
	data = protowire.AppendString(data, "conn-1")
	data = protowire.AppendTag(data, 6, protowire.BytesType)
	data = protowire.AppendString(data, "203.0.113.10:54321")
	data = protowire.AppendTag(data, 10, protowire.BytesType)
	data = protowire.AppendString(data, "user@example")
	data = protowire.AppendTag(data, 14, protowire.VarintType)
	data = protowire.AppendVarint(data, 1234)
	data = protowire.AppendTag(data, 15, protowire.VarintType)
	data = protowire.AppendVarint(data, 5678)

	got, err := decodeConnection(data, false)
	if err != nil {
		t.Fatalf("decodeConnection() error = %v", err)
	}
	if got.ID != "conn-1" || got.Source != "203.0.113.10:54321" || got.User != "user@example" {
		t.Fatalf("unexpected connection mapping: %+v", got)
	}
	if got.Uplink != 1234 || got.Downlink != 5678 {
		t.Fatalf("unexpected traffic mapping: up=%d down=%d", got.Uplink, got.Downlink)
	}
}

func TestDecodeConnectionEventMapping(t *testing.T) {
	connection := []byte{}
	connection = protowire.AppendTag(connection, 1, protowire.BytesType)
	connection = protowire.AppendString(connection, "conn-2")

	var data []byte
	data = protowire.AppendTag(data, 1, protowire.VarintType)
	data = protowire.AppendVarint(data, 1)
	data = protowire.AppendTag(data, 2, protowire.BytesType)
	data = protowire.AppendString(data, "conn-2")
	data = protowire.AppendTag(data, 3, protowire.BytesType)
	data = protowire.AppendBytes(data, connection)
	data = protowire.AppendTag(data, 4, protowire.VarintType)
	data = protowire.AppendVarint(data, 100)
	data = protowire.AppendTag(data, 5, protowire.VarintType)
	data = protowire.AppendVarint(data, 200)

	got, err := decodeConnectionEvent(data, false)
	if err != nil {
		t.Fatalf("decodeConnectionEvent() error = %v", err)
	}
	if got.Type != 1 || got.ID != "conn-2" || got.Connection == nil {
		t.Fatalf("unexpected event mapping: %+v", got)
	}
	if got.UplinkDelta != 100 || got.DownlinkDelta != 200 {
		t.Fatalf("unexpected event deltas: up=%d down=%d", got.UplinkDelta, got.DownlinkDelta)
	}
}
