package main

import "testing"

func TestExampleAliasCycleTerminates(t *testing.T) {
	gen := &exampleGen{aliasByName: map[string]Alias{
		"A": {Underlying: TypeRef{Kind: KindRef, Name: "B"}},
		"B": {Underlying: TypeRef{Kind: KindRef, Name: "A"}},
	}}
	if got := gen.forType(TypeRef{Kind: KindRef, Name: "A"}, map[string]bool{}); got != nil {
		t.Fatalf("cyclic alias example = %v, want nil", got)
	}
}

func TestGeneratedTrafficUsesCoreModels(t *testing.T) {
	schemas, aliases, err := walkPackages([]packageRequest{{
		Path: "../../internal/coremodel", StructAllow: setOf("ClientTraffic", "Traffic"), AliasAllow: setOf(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 0 || len(schemas) != 2 {
		t.Fatalf("traffic schemas = %d, aliases = %d", len(schemas), len(aliases))
	}
	gen := &exampleGen{}
	for _, schema := range schemas {
		example := gen.forSchema(schema, map[string]bool{})
		field := "up"
		if schema.Name == "Traffic" {
			field = "Up"
		}
		if example[field] != int64(1048576) {
			t.Fatalf("%s example omits traffic fields: %v", schema.Name, example)
		}
	}
}
