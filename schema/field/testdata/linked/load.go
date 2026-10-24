//go:build load

package main

// What entc's loader does before it encodes a schema, which lists the methods
// of each field's type: a call of reflect.Type.Method by index, on purpose.
// TestMakingFieldsLetsTheLinkerPruneMethods builds the program with this too,
// to see the linker say so.
func init() {
	for _, f := range (Schema{}).Fields() {
		f.Descriptor().Info.RType.LoadMethods()
	}
}
