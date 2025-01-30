package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type PointerComparisonFinder struct {
	fset   *token.FileSet
	issues []Issue
}

type Issue struct {
	pos     token.Position
	message string
}

func NewPointerComparisonFinder(fset *token.FileSet) *PointerComparisonFinder {
	return &PointerComparisonFinder{
		fset:   fset,
		issues: make([]Issue, 0),
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

func (v *PointerComparisonFinder) isPointerType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return true
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

		ast.Walk(finder, file)
		return nil
	})

	if err != nil {
		log.Fatalf("Error walking directory: %v", err)
	}

	for _, issue := range finder.issues {
		log.Printf("%s:%d: %s\n", issue.pos.Filename, issue.pos.Line, issue.message)
	}
}
