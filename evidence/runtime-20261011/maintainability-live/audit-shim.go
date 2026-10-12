package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func declarations(dir string) (map[string]string, error) {
	result := map[string]string{}
	fs := token.NewFileSet()
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		source, e := os.ReadFile(p)
		if e != nil {
			return nil, e
		}
		f, e := parser.ParseFile(fs, p, nil, parser.ParseComments)
		if e != nil {
			return nil, e
		}
		test := strings.HasSuffix(p, "_test.go")
		for _, d := range f.Decls {
			key := ""
			switch v := d.(type) {
			case *ast.FuncDecl:
				key = v.Name.Name
				if v.Recv != nil {
					var b bytes.Buffer
					format.Node(&b, fs, v.Recv.List[0].Type)
					key = b.String() + "." + key
				}
			case *ast.GenDecl:
				if v.Tok == token.IMPORT {
					continue
				}
				for _, s := range v.Specs {
					switch q := s.(type) {
					case *ast.TypeSpec:
						key += "type:" + q.Name.Name + ","
					case *ast.ValueSpec:
						for _, n := range q.Names {
							key += v.Tok.String() + ":" + n.Name + ","
						}
					}
				}
			}
			if test {
				key = "test:" + key
			}
			var b bytes.Buffer
			format.Node(&b, fs, d)
			if _, ok := result[key]; ok {
				return nil, fmt.Errorf("duplicate %s", key)
			}
			start := d.Pos()
			switch v := d.(type) {
			case *ast.FuncDecl:
				if v.Doc != nil {
					start = v.Doc.Pos()
				}
			case *ast.GenDecl:
				if v.Doc != nil {
					start = v.Doc.Pos()
				}
			}
			normalized, e := format.Source(append([]byte("package audit\n"), source[fs.Position(start).Offset:fs.Position(d.End()).Offset]...))
			if e != nil {
				return nil, e
			}
			result[key] = string(normalized)
		}
	}
	return result, nil
}
func symbols(m map[string]string) int {
	n := 0
	for k := range m {
		k = strings.TrimPrefix(k, "test:")
		if strings.HasPrefix(k, "type:") || strings.HasPrefix(k, "var:") || strings.HasPrefix(k, "const:") {
			n += strings.Count(k, ",")
		} else {
			n++
		}
	}
	return n
}
func main() {
	a, e := declarations(os.Args[1])
	if e != nil {
		panic(e)
	}
	b, e := declarations(os.Args[2])
	if e != nil {
		panic(e)
	}
	missing := []string{}
	changed := []string{}
	extra := []string{}
	for k, v := range a {
		w, ok := b[k]
		if !ok {
			missing = append(missing, k)
		} else if w != v {
			changed = append(changed, k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			extra = append(extra, k)
		}
	}
	r := map[string]any{"comparison": "standalone gofmt-normalized declaration text including all comments; import blocks excluded", "baseline_declaration_groups": len(a), "baseline_symbols": symbols(a), "current_declaration_groups": len(b), "current_symbols": symbols(b), "missing": missing, "changed": changed, "extra": extra}
	json.NewEncoder(os.Stdout).Encode(r)
	if len(missing)+len(changed)+len(extra) > 0 {
		os.Exit(1)
	}
}
