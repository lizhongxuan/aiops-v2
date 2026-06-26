package promptcompiler

import (
	"reflect"
	"testing"
)

func TestCompilerInterfaceExposesOnlyCompile(t *testing.T) {
	typ := reflect.TypeOf((*Compiler)(nil)).Elem()
	if _, ok := typ.MethodByName("Compile"); !ok {
		t.Fatal("Compiler missing Compile")
	}
	if _, ok := typ.MethodByName("CompileForEino"); ok {
		t.Fatal("Compiler must not expose CompileForEino")
	}
}
