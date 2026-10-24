// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package cobol

import (
	"errors"
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPositioned(t *testing.T) {
	t.Parallel()

	pos := Pos{Line: 7, Column: 12}
	tok := Token{Pos: pos, Type: TokenIdentifier, Value: []byte("X")}

	testCases := []struct {
		name string
		err  error
	}{
		{name: "unexpected character", err: UnexpectedCharacterError{Pos: pos, R: '~'}},
		{name: "unterminated string", err: UnterminatedStringError{Pos: pos}},
		{name: "unterminated pseudo-text", err: UnterminatedPseudoTextError{Pos: pos}},
		{name: "copybook not found", err: CopyBookNotFoundError{Pos: pos, Name: "CUST", Err: fs.ErrNotExist}},
		{name: "copybook cycle", err: CopyBookCycleError{Pos: pos, Name: "CUST", Stack: []string{"CUST", "CUST"}}},
		{name: "missing copybook resolver", err: MissingCopyBookResolverError{Pos: pos, Name: "CUST"}},
		{name: "invalid level number", err: InvalidLevelNumberError{Pos: pos, Value: "50"}},
		{name: "search all constraint", err: SearchAllConstraintError{Pos: pos, Reason: "two WHENs"}},
		{name: "unexpected token", err: UnexpectedTokenError{Expected: []TokenType{TokenSymbol}, Actual: tok}},
		{name: "unexpected keyword", err: UnexpectedKeywordError{Expected: []string{"PIC"}, Actual: tok}},
		{name: "trailing token", err: TrailingTokenError{Actual: tok}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wrapped := fmt.Errorf("reading copybook: %w", tc.err)

			var p Positioned
			require.ErrorAs(t, wrapped, &p)
			require.Equal(t, pos, p.Position())
			require.Equal(t, tc.err.Error(), p.Error())
		})
	}
}

// TestPositionedFromParse reaches the position of a real parse failure through
// one errors.As, without naming the error's type.
func TestPositionedFromParse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		src  string
		pos  Pos
	}{
		{
			name: "invalid level number",
			src:  "01 REC.\n   50 CUST-ID PIC X.\n",
			pos:  Pos{Line: 2, Column: 4},
		},
		{
			name: "trailing token",
			src:  "01 REC PIC X.\nPROCEDURE DIVISION.\n",
			pos:  Pos{Line: 2, Column: 1},
		},
		{
			name: "unterminated string",
			src:  "01 REC PIC X VALUE 'OPEN.\n",
			pos:  Pos{Line: 1, Column: 20},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := Parse(strings.NewReader(tc.src), WithFragment())
			wrapped := fmt.Errorf("parsing copybook: %w", err)

			var p Positioned
			require.ErrorAs(t, wrapped, &p)
			require.Equal(t, tc.pos, p.Position())
		})
	}
}

// TestEveryPositionedErrorHasPosition reads every package of this module and
// fails for an exported error type that carries a source position — a field of
// type Pos or Token, cobol.Pos or cobol.Token outside the root package — and
// has no Position method, so an error type added later cannot miss it. It also
// fails for one that carries no position and has the method anyway, which is
// what keeps "errors with no source position are unchanged" true.
func TestEveryPositionedErrorHasPosition(t *testing.T) {
	t.Parallel()

	types := scanErrorTypes(t, ".")

	var positioned []string
	for _, et := range types {
		if et.carriesPos {
			positioned = append(positioned, et.name)
			require.Truef(t, et.hasPosition, "%s carries a source position and has no Position method", et.name)
			continue
		}
		require.Falsef(t, et.hasPosition, "%s carries no source position and has a Position method", et.name)
	}

	// Guard the guard: a scan that found nothing would pass vacuously.
	require.Contains(t, positioned, "UnexpectedTokenError")
	require.Contains(t, positioned, "InvalidLevelNumberError")
	require.Contains(t, positioned, "copybook.LevelError")
	require.Contains(t, positioned, "copybook.DependingError")
	require.NotContains(t, positioned, "codec.OffsetError")
}

// errorType is one exported type, declared in a package of this module, that has
// an Error method.
type errorType struct {
	name        string // package-qualified outside the root package
	carriesPos  bool
	hasPosition bool
}

// scanErrorTypes parses the non-test Go files of every package under root,
// skipping hidden directories (worktrees live under .claude) and testdata.
func scanErrorTypes(t *testing.T, root string) []errorType {
	t.Helper()

	var out []errorType
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata") {
			return filepath.SkipDir
		}
		types, err := scanPackageErrorTypes(path)
		if err != nil {
			return err
		}
		out = append(out, types...)
		return nil
	})
	require.NoError(t, err)
	return out
}

func scanPackageErrorTypes(dir string) ([]errorType, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	structs := map[string]*ast.StructType{}
	methods := map[string]map[string]bool{}
	var pkg string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := goparser.ParseFile(fset, filepath.Join(dir, name), nil, goparser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		pkg = f.Name.Name
		for _, decl := range f.Decls {
			switch decl := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok || !ts.Name.IsExported() {
						continue
					}
					if st, ok := ts.Type.(*ast.StructType); ok {
						structs[ts.Name.Name] = st
					}
				}
			case *ast.FuncDecl:
				if decl.Recv == nil || len(decl.Recv.List) != 1 {
					continue
				}
				recv := receiverName(decl.Recv.List[0].Type)
				if methods[recv] == nil {
					methods[recv] = map[string]bool{}
				}
				methods[recv][decl.Name.Name] = true
			}
		}
	}

	var out []errorType
	for name, st := range structs {
		if !methods[name]["Error"] {
			continue
		}
		qualified := name
		if pkg != "cobol" {
			qualified = pkg + "." + name
		}
		out = append(out, errorType{
			name:        qualified,
			carriesPos:  carriesPos(st),
			hasPosition: methods[name]["Position"],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

func receiverName(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// carriesPos reports whether a struct has a field of type Pos or Token (or a
// pointer to one), in the root package or as cobol.Pos / cobol.Token.
func carriesPos(st *ast.StructType) bool {
	for _, field := range st.Fields.List {
		typ := field.Type
		if star, ok := typ.(*ast.StarExpr); ok {
			typ = star.X
		}
		var name string
		switch typ := typ.(type) {
		case *ast.Ident:
			name = typ.Name
		case *ast.SelectorExpr:
			if x, ok := typ.X.(*ast.Ident); ok && x.Name == "cobol" {
				name = typ.Sel.Name
			}
		}
		if name == "Pos" || name == "Token" {
			return true
		}
	}
	return false
}

func TestPositionedErrorsWithoutPosition(t *testing.T) {
	t.Parallel()

	var p Positioned
	require.False(t, errors.As(UnexpectedEndOfTokensError{}, &p), "UnexpectedEndOfTokensError carries no position")
	require.False(t, errors.As(UnsupportedNodeError{}, &p), "UnsupportedNodeError carries no position")
}
