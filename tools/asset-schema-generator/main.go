// Command asset-schema-generator writes the typed catalog persistence projection.
// Run from the repository root after adding fields to assets.Catalog.
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"reflect"
	"strings"
	"wonderland-go/internal/assets"
)

type key struct{ name, typ string }
type node struct {
	name, table, typ, kind string
	keys                   []key
	keyType                string
	fields, write, read    []string
	children               []*node
	sourceExpression       string
	valueType              string
}

var nodes []*node

func line(dst *[]string, f string, a ...any) { *dst = append(*dst, fmt.Sprintf(f, a...)) }
func typ(t reflect.Type) string              { return t.String() }
func kind(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Map:
		return "map"
	case reflect.Slice:
		return "slice"
	default:
		return "single"
	}
}
func snake(s string) string {
	var b strings.Builder
	for i, c := range s {
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(c + 'a' - 'A')
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}
func create(name string, t reflect.Type, parent *node) *node {
	n := &node{name: name, table: "catalog_" + snake(name), typ: typ(t), kind: kind(t)}
	if parent != nil {
		n.keys = append(n.keys, parent.keys...)
	}
	value := t
	switch n.kind {
	case "map":
		n.keyType = typ(t.Key())
		n.keys = append(n.keys, key{name + "Key", n.keyType})
		value = t.Elem()
	case "slice":
		n.keys = append(n.keys, key{name + "Ordinal", "int"})
		value = t.Elem()
	case "single":
		if parent == nil {
			n.keys = append(n.keys, key{name + "Key", "int"})
		}
	}
	n.valueType = typ(value)
	nodes = append(nodes, n)
	for _, k := range n.keys {
		line(&n.fields, "%s %s `gorm:\"column:%s;primaryKey;autoIncrement:false\"`", k.name, k.typ, snake(k.name))
	}
	if parent != nil {
		names := []string{}
		for _, k := range parent.keys {
			names = append(names, k.name)
		}
		line(&n.fields, "Parent *%sRow `gorm:\"belongsTo:Parent;foreignKey:%s;references:%s;constraint:OnDelete:CASCADE\"`", parent.name, strings.Join(names, ","), strings.Join(names, ","))
	}
	flatten(n, value, "value", "Value")
	return n
}
func flatten(n *node, t reflect.Type, expr, col string) {
	switch t.Kind() {
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				panic("unexported field " + f.Name)
			}
			flatten(n, f.Type, expr+"."+f.Name, col+f.Name)
		}
	case reflect.Pointer:
		line(&n.fields, "%sPresent bool", col)
		line(&n.write, "if %s!=nil { row.%sPresent=true", expr, col)
		line(&n.read, "if row.%sPresent { %s=new(%s)", col, expr, typ(t.Elem()))
		flatten(n, t.Elem(), "(*"+expr+")", col)
		line(&n.write, "}")
		line(&n.read, "}")
	case reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			line(&n.fields, "%s []byte", col)
			line(&n.write, "row.%s=append([]byte(nil),%s[:]...)", col, expr)
			line(&n.read, "if len(row.%s)!=len(%s){return result,fmt.Errorf(\"invalid %s byte length\")};copy(%s[:],row.%s)", col, expr, col, expr, col)
		} else {
			for i := 0; i < t.Len(); i++ {
				flatten(n, t.Elem(), fmt.Sprintf("%s[%d]", expr, i), fmt.Sprintf("%s%d", col, i))
			}
		}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			line(&n.fields, "%s []byte", col)
			line(&n.fields, "%sPresent bool", col)
			line(&n.write, "row.%s=%s;row.%sPresent=%s!=nil", col, expr, col, expr)
			line(&n.read, "if row.%sPresent { %s=append([]byte{},row.%s...) }", col, expr, col)
		} else {
			nested(n, t, expr, col)
		}
	case reflect.Map:
		nested(n, t, expr, col)
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		line(&n.fields, "%s %s", col, typ(t))
		line(&n.write, "row.%s=%s", col, expr)
		line(&n.read, "%s=row.%s", expr, col)
	default:
		panic(fmt.Sprintf("unsupported %s", t))
	}
}
func nested(n *node, t reflect.Type, expr, col string) {
	suffix := strings.TrimPrefix(col, "Value")
	if suffix == "" {
		suffix = "Values"
	}
	child := create(n.name+suffix, t, n)
	child.sourceExpression = expr
	n.children = append(n.children, child)
	line(&n.fields, "%sPresent bool", col)
	args := []string{}
	for _, k := range n.keys {
		args = append(args, "row."+k.name)
	}
	line(&n.write, "row.%sPresent=%s!=nil", col, expr)
	// Children are written after the parent batch has been inserted.
	line(&n.read, "if row.%sPresent { var err error;%s,err=read%s(tx,%s);if err!=nil{return result,err} }", col, expr, child.name, strings.Join(args, ","))
}
func params(keys []key) string {
	out := []string{}
	for _, k := range keys {
		out = append(out, k.name+" "+k.typ)
	}
	return strings.Join(out, ",")
}
func main() {
	catalog := reflect.TypeOf(assets.Catalog{})
	roots := []*node{}
	names := []string{}
	for i := 0; i < catalog.NumField(); i++ {
		f := catalog.Field(i)
		if f.Name == "AssetsDatabase" || f.Name == "ItemCatalog" || f.Name == "Items" {
			continue
		}
		roots = append(roots, create(f.Name, f.Type, nil))
		names = append(names, f.Name)
	}
	var b bytes.Buffer
	fmt.Fprintln(&b, "// Code generated by tools/asset-schema-generator; DO NOT EDIT.\npackage assetsql\nimport(\"fmt\";\"gorm.io/gorm\";\"wonderland-go/internal/assets\";\"wonderland-go/internal/game\")")
	for _, n := range nodes {
		fmt.Fprintf(&b, "type %sRow struct {\n%s\n}\nfunc (%sRow)TableName()string{return %q}\n", n.name, strings.Join(n.fields, "\n"), n.name, n.table)
		inherited := n.keys[:len(n.keys)-1]
		if n.kind == "single" {
			inherited = n.keys[:0]
		}
		args := params(inherited)
		if args != "" {
			args = "," + args
		}
		fmt.Fprintf(&b, "func write%s(tx *gorm.DB, values %s%s)error{\nvar rows []%sRow\n", n.name, n.typ, args, n.name)
		switch n.kind {
		case "map", "slice":
			fmt.Fprintln(&b, "for key,value:=range values {")
		case "single":
			fmt.Fprintln(&b, "{\nkey,value:=1,values")
		}
		fmt.Fprintf(&b, "row:=%sRow{", n.name)
		for _, k := range inherited {
			fmt.Fprintf(&b, "%s:%s,", k.name, k.name)
		}
		last := n.keys[len(n.keys)-1]
		fmt.Fprintf(&b, "%s:key}\n", last.name)
		fmt.Fprintln(&b, strings.Join(n.write, "\n"))
		fmt.Fprintln(&b, "rows=append(rows,row)\n}\nif len(rows)>0{if err:=tx.CreateInBatches(&rows,catalogWriteBatch).Error;err!=nil{return err}}")
		if len(n.children) > 0 {
			switch n.kind {
			case "map", "slice":
				fmt.Fprintln(&b, "for key,value:=range values {")
			case "single":
				fmt.Fprintln(&b, "{\nkey,value:=1,values")
			}
			for _, ch := range n.children {
				expression := ch.sourceExpression
				keys := []string{}
				for _, k := range inherited {
					keys = append(keys, k.name)
				}
				keys = append(keys, "key")
				fmt.Fprintf(&b, "if err:=write%s(tx,%s,%s);err!=nil{return err}\n", ch.name, expression, strings.Join(keys, ","))
			}
			fmt.Fprintln(&b, "}")
		}
		fmt.Fprintln(&b, "return nil\n}")
		fmt.Fprintf(&b, "func read%s(tx *gorm.DB%s)(%s,error){\n", n.name, args, n.typ)
		switch n.kind {
		case "map":
			fmt.Fprintf(&b, "result:=make(%s)\n", n.typ)
		case "slice":
			fmt.Fprintf(&b, "result:=make(%s,0)\n", n.typ)
		case "single":
			fmt.Fprintf(&b, "var result %s\n", n.typ)
		}
		fmt.Fprintf(&b, "var rows []%sRow\nquery:=tx\n", n.name)
		for _, k := range inherited {
			fmt.Fprintf(&b, "query=query.Where(%q,%s)\n", snake(k.name)+" = ?", k.name)
		}
		fmt.Fprintf(&b, "if err:=query.Order(%q).Find(&rows).Error;err!=nil{return result,err}\n", snake(last.name))
		if n.kind == "single" {
			fmt.Fprintln(&b, "if len(rows)!=1{return result,fmt.Errorf(\"missing singleton catalog row\")}")
		}
		fmt.Fprintln(&b, "for _,row:=range rows {")
		valueType := n.valueType
		fmt.Fprintf(&b, "var value %s\n", valueType)
		fmt.Fprintln(&b, strings.Join(n.read, "\n"))
		switch n.kind {
		case "map":
			fmt.Fprintf(&b, "result[row.%s]=value\n", last.name)
		case "slice":
			fmt.Fprintf(&b, "if row.%s!=len(result){return result,fmt.Errorf(\"invalid catalog ordinal\")};result=append(result,value)\n", last.name)
		case "single":
			fmt.Fprintln(&b, "result=value")
		}
		fmt.Fprintln(&b, "}\nreturn result,nil\n}")
	}
	fmt.Fprintln(&b, "type catalogPresence struct { ID int `gorm:\"primaryKey;autoIncrement:false\"`")
	for _, n := range roots {
		if n.kind != "single" {
			fmt.Fprintf(&b, "%s bool\n", n.name)
		}
	}
	fmt.Fprintln(&b, "}\nfunc(catalogPresence)TableName()string{return \"catalog_presence\"}")
	fmt.Fprintln(&b, "func catalogTables()[]any{return []any{&catalogPresence{},")
	for _, n := range nodes {
		fmt.Fprintf(&b, "&%sRow{},\n", n.name)
	}
	fmt.Fprintln(&b, "}}")
	fmt.Fprintln(&b, "func writeCatalog(tx *gorm.DB,c *assets.Catalog)error{")
	for i := len(nodes) - 1; i >= 0; i-- {
		fmt.Fprintf(&b, "if err:=tx.Where(\"1 = 1\").Delete(&%sRow{}).Error;err!=nil{return err}\n", nodes[i].name)
	}
	fmt.Fprintln(&b, "if err:=tx.Where(\"1 = 1\").Delete(&catalogPresence{}).Error;err!=nil{return err}\npresence:=catalogPresence{ID:catalogMetadataID}")
	for _, n := range roots {
		if n.kind != "single" {
			fmt.Fprintf(&b, "presence.%s=c.%s!=nil\n", n.name, n.name)
		}
	}
	fmt.Fprintln(&b, "if err:=tx.Create(&presence).Error;err!=nil{return err}")
	for i, n := range roots {
		fmt.Fprintf(&b, "if err:=write%s(tx,c.%s);err!=nil{return err}\n", n.name, names[i])
	}
	fmt.Fprintln(&b, "return nil\n}")
	fmt.Fprintln(&b, "func readCatalog(tx *gorm.DB)(*assets.Catalog,error){c:=&assets.Catalog{};var err error")
	for i, n := range roots {
		fmt.Fprintf(&b, "c.%s,err=read%s(tx);if err!=nil{return nil,err}\n", names[i], n.name)
	}
	fmt.Fprintln(&b, "var presence catalogPresence;if err:=tx.First(&presence,catalogMetadataID).Error;err!=nil{return nil,err}")
	for _, n := range roots {
		if n.kind != "single" {
			fmt.Fprintf(&b, "if !presence.%s {c.%s=nil}\n", n.name, n.name)
		}
	}
	fmt.Fprintln(&b, "c.Items=make(map[uint16]game.ItemDefinition,len(c.NativeItems));for id,item:=range c.NativeItems{c.Items[id]=item.Definition}")
	fmt.Fprintln(&b, "return c,nil\n}")
	out, err := format.Source(b.Bytes())
	if err != nil {
		os.WriteFile("/tmp/catalog-generated-invalid.go", b.Bytes(), 0600)
		panic(err)
	}
	if err = os.WriteFile("internal/assetsql/catalog_storage_generated.go", out, 0644); err != nil {
		panic(err)
	}
}
