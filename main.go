package main

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type PointerComparisonFinder struct {
	fset   *token.FileSet
	issues []Issue
	info   *types.Info
	conf   types.Config
}

type Issue struct {
	pos     token.Position
	message string
}

func NewPointerComparisonFinder(fset *token.FileSet) *PointerComparisonFinder {
	return &PointerComparisonFinder{
		fset:   fset,
		issues: make([]Issue, 0),
		info: &types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
		},
		conf: types.Config{
			Importer: importer.Default(),
			Error: func(err error) {
				log.Printf("DEBUG: Type checker error: %v", err)
			},
		},
	}
}

func (v *PointerComparisonFinder) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}

	if binaryExpr, ok := node.(*ast.BinaryExpr); ok {
		switch binaryExpr.Op {
		case token.EQL, token.NEQ, token.LSS, token.GTR, token.LEQ, token.GEQ:
			if v.isPointerType(binaryExpr.X) && v.isPointerType(binaryExpr.Y) {
				leftType := v.getUnderlyingType(binaryExpr.X)
				rightType := v.getUnderlyingType(binaryExpr.Y)
				if !(isBasicType(leftType) || isBasicType(rightType)) {
					return v
				}

				pos := v.fset.Position(binaryExpr.Pos())
				v.issues = append(v.issues, Issue{
					pos:     pos,
					message: "Direct pointer comparison found. Consider comparing the dereferenced values instead.",
				})
			}
		}
	}

	return v
}

func (v *PointerComparisonFinder) getUnderlyingType(expr ast.Expr) types.Type {
	tv, ok := v.info.Types[expr]
	if !ok {
		return nil
	}

	if ptr, ok := tv.Type.(*types.Pointer); ok {
		underlying := ptr.Elem()
		return underlying
	}

	return tv.Type
}

func isBasicType(t types.Type) bool {
	if _, ok := t.(*types.Basic); ok {
		return true
	}
	return false
}

func (v *PointerComparisonFinder) isPointerType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return false
	case *ast.Ident:
		if t.Obj != nil && t.Obj.Decl != nil {
			if valueSpec, ok := t.Obj.Decl.(*ast.ValueSpec); ok {
				if valueSpec.Type != nil {
					_, isPtr := valueSpec.Type.(*ast.StarExpr)
					return isPtr
				}
			}
		}
	case *ast.SelectorExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return strings.HasSuffix(ident.Name, "Ptr")
		}
	}
	return false
}

func (v *PointerComparisonFinder) checkFile(_ string, file *ast.File) error {
	files := []*ast.File{file}

	conf := &types.Config{
		Importer:    importer.Default(),
		FakeImportC: true,
		Error: func(err error) {
			log.Printf("DEBUG: Type checker error: %v", err)
		},
	}

	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}

	pkgName := file.Name.Name

	_, err := conf.Check(pkgName, v.fset, files, info)
	if err != nil {
		return err
	}

	// Store the type info for use in Visit
	v.info = info

	ast.Walk(v, file)
	return nil
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: ptrcomp <directory>")
	}
	dir := os.Args[1]

	fset := token.NewFileSet()
	finder := NewPointerComparisonFinder(fset)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && !strings.HasSuffix(path, ".go") {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			log.Printf("Failed to parse %s: %v\n", path, err)
			return nil
		}

		if err := finder.checkFile(path, file); err != nil {
			log.Printf("Failed to type check %s: %v\n", path, err)
			return nil
		}
		return nil
	})

	if err != nil {
		log.Fatalf("Error walking directory: %v", err)
	}

	for _, issue := range finder.issues {
		log.Printf("%s:%d: %s\n", issue.pos.Filename, issue.pos.Line, issue.message)
	}
}
