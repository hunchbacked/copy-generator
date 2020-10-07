package gen

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
)

const (
	InPrefix  = "in."
	OutPrefix = "out."
)

type structField reflect.StructField

type generator struct {
	// package path to local alias map for tracking imports
	imports map[string]string

	// types in queue
	types map[reflect.Type]bool

	varIndex int64
	pkgName  string
	pkgPath  string
}

// NewGenerator initializes and returns a generator.
func NewGenerator() *generator {
	ret := &generator{
		imports: map[string]string{},
		types:   make(map[reflect.Type]bool),
	}

	return ret
}

func (g *generator) SetPkg(name, path string) {
	g.pkgName = name
	g.pkgPath = path
}

func (g *generator) Add(obj interface{}) {
	t := reflect.TypeOf(obj)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	g.addType(t)
}

func (g *generator) addType(t reflect.Type) {
	if g.pkgPath != t.PkgPath() {
		return
	}

	if _, ok := g.types[t]; ok {
		return
	}
	g.types[t] = false
}

func (g *generator) nextType() reflect.Type {
	for t := range g.types {
		if g.types[t] == false {
			g.types[t] = true
			return t
		}
	}

	return nil
}

// printHeader prints package declaration and imports.
func (g *generator) printHeader() {
	fmt.Println("// Code generated by rti-generator for copy struct. DO NOT EDIT.")
	fmt.Println()
	fmt.Println("package ", g.pkgName)
	fmt.Println()

	byAlias := map[string]string{}
	var aliases []string
	for path, alias := range g.imports {
		aliases = append(aliases, alias)
		byAlias[alias] = path
	}

	sort.Strings(aliases)
	fmt.Println("import (")
	for _, alias := range aliases {
		fmt.Printf("  %s %q\n", alias, byAlias[alias])
	}

	fmt.Println(")")
	fmt.Println()
}

func (g generator) isPtr(p reflect.Type) bool {
	if p.Kind() == reflect.Ptr {
		return true
	}
	return false
}

func (g generator) isSlice(p reflect.Type) bool {
	if p.Kind() == reflect.Slice {
		return true
	}
	return false
}

func (g *generator) nextVar() string {
	g.varIndex++
	return fmt.Sprintf("v%d", g.varIndex)
}

/*
	Bool
	Int
	Int8
	Int16
	Int32
	Int64
	Uint
	Uint8
	Uint16
	Uint32
	Uint64
	Uintptr
	Float32
	Float64
	Complex64
	Complex128
	String

	???
	Chan
	Func
*/

func (g generator) isBaseType(k reflect.Kind) bool {
	if k != reflect.Slice && k != reflect.Map && k != reflect.Ptr && k != reflect.Struct && k != reflect.Array {
		return true
	}
	return false
}

func (s *structField) in() string {
	return InPrefix + s.Name
}

func (s *structField) out() string {
	return OutPrefix + s.Name
}

func (g *generator) _object(t reflect.Type) {
	if t.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < t.NumField(); i++ {
		f := structField(t.Field(i))
		g.field(f)
	}

	return
}

func (g *generator) _struct(f structField) {
	if f.Type.Kind() != reflect.Struct {
		return
	}

	if f.Type.PkgPath() == g.pkgPath {
		g.addType(f.Type)
		fmt.Printf("%s = %s.Copy()\n", f.out(), f.in())
	} else {
		fmt.Printf("%s = %s\n", f.out(), f.in())
	}

	return
}

func (g *generator) _ptrStruct(f structField) {
	if f.Type.Kind() != reflect.Struct {
		return
	}

	v := g.nextVar()

	if f.Type.PkgPath() == g.pkgPath {
		g.addType(f.Type)
		fmt.Printf("%s := %s.Copy()\n%s = &%s\n", v, f.in(), f.out(), v)
	} else {
		fmt.Printf("%s := *%s\n%s = &%s\n", v, f.in(), f.out(), v)
	}

	return
}

func (g *generator) _ptr(f structField) {
	if f.Type.Kind() != reflect.Ptr {
		return
	}

	e := f.Type.Elem()
	sf := structField{Type: e, Name: f.Name}

	fmt.Printf("if %s != nil {\n", f.in())

	if g.isBaseType(e.Kind()) {
		g._ptrBaseType(sf)
	}

	if e.Kind() == reflect.Struct {
		g._ptrStruct(sf)
	}

	fmt.Println("}")

	return
}

func (g *generator) _sliceItem(f structField, index string) {
	if f.Type.Kind() != reflect.Slice && f.Type.Kind() != reflect.Array {
		return
	}

	e := f.Type.Elem()
	g.field(structField{Type: e, Name: f.Name + "[" + index + "]"})

	return
}

func (g *generator) _slice(f structField) {
	if f.Type.Kind() != reflect.Slice && f.Type.Kind() != reflect.Array {
		return
	}

	ptr := ""
	e := f.Type.Elem()
	if e.Kind() == reflect.Ptr {
		ptr = "*"
		e = e.Elem()
	}

	needEnd := false
	if f.Type.Kind() == reflect.Slice {
		fmt.Printf("if len(%s) > 0 {\n", f.out())
		needEnd = true

		typ := e.Name()
		if e.Kind() == reflect.Interface {
			typ = "interface{}"
		}
		if e.PkgPath() != g.pkgPath {
			typ = e.String()
		}

		fmt.Printf("%s = make([]%s%s, len(%s))\n", f.out(), ptr, typ, f.in())
	}

	lv := g.nextVar()

	fmt.Printf("for %s := range %s {\n", lv, f.in())
	g._sliceItem(f, lv)
	fmt.Println("}")
	if needEnd {
		fmt.Println("}")
	}
	
	return
}

func (g *generator) _baseType(f structField) {
	if !g.isBaseType(f.Type.Kind()) {
		return
	}

	fmt.Printf("%s = %s\n", f.out(), f.in())

	return
}

func (g *generator) _ptrBaseType(f structField) {
	if !g.isBaseType(f.Type.Kind()) {
		return
	}

	v := g.nextVar()

	fmt.Printf("%s := *%s\n%s = &%s\n", v, f.in(), f.out(), v)

	return
}

func (g *generator) field(f structField) {

	if noCopy, _ := strconv.ParseBool(f.Tag.Get("noCopy")); noCopy {
		return
	}

	g._baseType(f)
	g._slice(f)
	g._ptr(f)
	g._struct(f)

	return
}

// Run runs the generator and outputs generated code to out.
func (g *generator) Run(out io.Writer) error {
	g.printHeader()

	for t := g.nextType(); t != nil; t = g.nextType() {
		fmt.Printf("func (in *%s) Copy() (out %s) {\n", t.Name(), t.Name())
		g._object(t)
		fmt.Println("return")
		fmt.Println("}")
	}

	return nil
}
