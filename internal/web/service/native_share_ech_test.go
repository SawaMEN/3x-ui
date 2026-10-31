package service

import (
    "net/url"
    "reflect"
    "testing"
)

func TestNativeImportECHQueryFormats(t *testing.T) {
    cases := []struct {
        name string
        query url.Values
        want map[string]any
    }{
        {"uri ECH config", url.Values{"ech": {"AQIDBA"}}, map[string]any{"enabled": true, "config": []string{"AQIDBA"}}},
        {"explicit ECH config", url.Values{"tls.ech": {"AQIDBA"}}, map[string]any{"enabled": true, "config": []string{"AQIDBA"}}},
        {"disable ECH", url.Values{"ech": {"false"}}, map[string]any{"enabled": false}},
        {"ECH JSON", url.Values{"tls.ech": {`{"enabled":true,"query_server_name":"cloud.example"}`}}, map[string]any{"enabled": true, "query_server_name": "cloud.example"}},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            out := map[string]any{"type": "anytls", "tls": map[string]any{"enabled": true}}
            if err := applyNativeShareOptions(out, tc.query); err != nil {
                t.Fatal(err)
            }
            got := out["tls"].(map[string]any)["ech"]
            if !reflect.DeepEqual(got, tc.want) {
                t.Fatalf("ECH = %#v, want %#v", got, tc.want)
            }
        })
    }
}

func TestNativeImportECHMergesExistingNativeSettings(t *testing.T) {
    out := map[string]any{"type": "anytls", "tls": map[string]any{"ech": map[string]any{"enabled": true, "query_server_name": "test.example"}}}
    if err := applyNativeShareOptions(out, url.Values{"ech": {"AQIDBA"}}); err != nil {
        t.Fatal(err)
    }
    ech := out["tls"].(map[string]any)["ech"].(map[string]any)
    if ech["query_server_name"] != "test.example" || !reflect.DeepEqual(ech["config"], []string{"AQIDBA"}) {
        t.Fatalf("existing ECH options were lost: %#v", ech)
    }
    if err := applyNativeShareOptions(out, url.Values{"ech": {`{"enabled":`}}); err == nil {
        t.Fatal("accepted malformed ECH object")
    }
}
