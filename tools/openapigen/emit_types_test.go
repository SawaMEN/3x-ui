package main

import "testing"

func TestTSArrayOfNullableRefs(t *testing.T) {
	element := TypeRef{Kind: KindRef, Name: "Result"}
	pointer := TypeRef{Kind: KindRef, Name: "nullable", Inner: &element}
	if got := tsTypeExpr(TypeRef{Kind: KindArray, Element: &pointer}); got != "(Result | null)[]" {
		t.Fatal(got)
	}
}
