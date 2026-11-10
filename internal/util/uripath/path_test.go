package uripath

import "testing"

func TestNormalize(t *testing.T) {
	for input, want := range map[string]string{"": "/", "/": "/", "///": "/", "secret": "/secret/", "//secret///child//": "/secret/child/", "/путь/": "/путь/"} {
		got, err := Normalize(input)
		if err != nil || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"/./", "/a/../b/", "/secret#part/", "/:param/", "/secret%2f/", "https://example.com/", "/has space/", "/bad;path/"} {
		if _, err := Normalize(input); err == nil {
			t.Errorf("accepted invalid path %q", input)
		}
	}
}

func TestUnicodeCookiePath(t *testing.T) {
	if got := CookiePath("/панель/"); got != "/%D0%BF%D0%B0%D0%BD%D0%B5%D0%BB%D1%8C/" {
		t.Fatalf("cookie path %q", got)
	}
	if got := CookiePath("/plain/path/"); got != "/plain/path/" {
		t.Fatalf("plain cookie path %q", got)
	}
}
