package sudoku

import "testing"

func TestValidPrivateKey(t *testing.T) {
    if !ValidPrivateKey("zz") {
        t.Fatal("expected invalid key")
    }
    valid64 := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    if !ValidPrivateKey(valid64) {
        t.Fatal("expected master key to be valid")
    }
    if !ValidPrivateKey(valid64+valid64) {
        t.Fatal("expected split key to be valid")
    }
}
