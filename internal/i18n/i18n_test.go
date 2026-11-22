package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestCatalogComplete fails if a literal passed to T or Err has no English translation.
func TestCatalogComplete(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	missing := map[string]string{}
	checked := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok || len(c.Args) == 0 {
				return true
			}
			sel, ok := c.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "T" && sel.Sel.Name != "Err" && sel.Sel.Name != "N") {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); !ok || id.Name != "i18n" {
				return true
			}
			if bl, ok := c.Args[0].(*ast.BasicLit); ok && bl.Kind == token.STRING {
				s, _ := strconv.Unquote(bl.Value)
				checked++
				if !Has(s) {
					missing[s] = path
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 150 {
		t.Fatalf("scanner found only %d translated literals", checked)
	}
	for s, p := range missing {
		t.Errorf("no English translation for %q (%s)", s, p)
	}
}

func TestLanguages(t *testing.T) {
	defer SetLang(EN)
	SetLang(RU)
	if got := T("Сохранить"); got != "Сохранить" {
		t.Fatalf("ru: %q", got)
	}
	SetLang(EN)
	if got := T("Сохранить"); got != "Save" {
		t.Fatalf("en: %q", got)
	}
	if got := T("Качество: %d", 3); got != "Quality: 3" {
		t.Fatalf("format: %q", got)
	}
	if got := T("совсем нового текста нет"); got != "совсем нового текста нет" {
		t.Fatalf("fallback: %q", got)
	}
}
