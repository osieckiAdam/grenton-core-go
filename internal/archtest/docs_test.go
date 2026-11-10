package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// publicPackages are the packages applications import.
var publicPackages = []string{"", "model", "clu", "devices", "provisioning"}

// implementsDocumented are method names that implement a documented
// interface method of this module or the standard library; their contract
// is the interface's.
var implementsDocumented = map[string]bool{
	"Info": true, "Updates": true, "WidgetID": true, "WidgetType": true, "String": true, "Error": true,
	"Is": true, "Datagram": true, "Closed": true, "Stop": true, "Now": true, "AfterFunc": true,
}

// Every exported identifier of a public package is documented.
func TestPublicAPIIsDocumented(t *testing.T) {
	for _, rel := range publicPackages {
		dir := filepath.Join("..", "..", rel)
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, dir, func(fi fs.FileInfo) bool {
			return !strings.HasSuffix(fi.Name(), "_test.go")
		}, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		for _, pkg := range pkgs {
			hasPackageDoc := false
			for name, f := range pkg.Files {
				if f.Doc != nil {
					hasPackageDoc = true
				}
				check(t, fset, name, f)
			}
			if !hasPackageDoc {
				t.Errorf("package %s has no package comment", pkg.Name)
			}
		}
	}
}

func check(t *testing.T, fset *token.FileSet, file string, f *ast.File) {
	report := func(pos token.Pos, what string) {
		t.Errorf("%s: %s is exported but undocumented", fset.Position(pos), what)
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if !d.Name.IsExported() || d.Doc != nil {
				continue
			}
			if d.Recv != nil {
				if !receiverExported(d.Recv) || implementsDocumented[d.Name.Name] {
					continue
				}
			}
			report(d.Pos(), d.Name.Name)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name.IsExported() && d.Doc == nil && s.Doc == nil {
						report(s.Pos(), s.Name.Name)
					}
				case *ast.ValueSpec:
					for _, n := range s.Names {
						if n.IsExported() && d.Doc == nil && s.Doc == nil && s.Comment == nil {
							report(n.Pos(), n.Name)
						}
					}
				}
			}
		}
	}
}

func receiverExported(fl *ast.FieldList) bool {
	if len(fl.List) == 0 {
		return false
	}
	expr := fl.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	id, ok := expr.(*ast.Ident)
	return ok && id.IsExported()
}
