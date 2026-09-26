from pathlib import Path


def once(text: str, old: str, new: str, label: str) -> str:
    n = text.count(old)
    if n != 1:
        raise SystemExit(f"{label}: expected one match, found {n}")
    return text.replace(old, new, 1)


# main already registers these external-VPN routes; keep the API registry in sync.
path = Path("frontend/src/pages/api-docs/endpoints.ts")
text = path.read_text()
if "/panel/api/server/externalvpn/status" not in text:
    marker = "export const sections: readonly Section[] = [\n"
    section = """  {
    id: 'externalvpn',
    title: 'External VPN',
    description: 'Inspect and update standalone external VPN components.',
    endpoints: [
      {
        method: 'GET',
        path: '/panel/api/server/externalvpn/status',
        summary: 'Return external VPN component installation and update status.',
      },
      {
        method: 'POST',
        path: '/panel/api/server/externalvpn/update/:protocol',
        summary: 'Install or update an external VPN component.',
        params: [
          {
            name: 'protocol',
            in: 'path',
            type: 'string',
            desc: 'External VPN protocol identifier.',
          },
        ],
      },
    ],
  },
"""
    text = once(text, marker, marker + section, "external VPN API docs")
path.write_text(text)


# TranslateXrayOutbound normalizes Hysteria bandwidth through rawInt, so the
# resulting values are ints. The previous test asserted float64 even though the
# production translator intentionally emits normalized integer Mbps values.
path = Path("internal/sub/json_service_test.go")
text = path.read_text()
text = once(
    text,
    'if up, ok := got["up_mbps"].(float64); !ok || up != 100 {\n\t\tt.Fatalf("up_mbps = %v, want float64(100)", got["up_mbps"])\n\t}',
    'if up, ok := got["up_mbps"].(int); !ok || up != 100 {\n\t\tt.Fatalf("up_mbps = %v, want int(100)", got["up_mbps"])\n\t}',
    "Hysteria2 up_mbps test type",
)
text = once(
    text,
    'if down, ok := got["down_mbps"].(float64); !ok || down != 50 {\n\t\tt.Fatalf("down_mbps = %v, want float64(50)", got["down_mbps"])\n\t}',
    'if down, ok := got["down_mbps"].(int); !ok || down != 50 {\n\t\tt.Fatalf("down_mbps = %v, want int(50)", got["down_mbps"])\n\t}',
    "Hysteria2 down_mbps test type",
)
path.write_text(text)
