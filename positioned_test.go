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
		{name: "entry check", err: EntryCheckError{Index: 2, Pos: pos, Printed: "01 A PIC X.\n", Err: ErrEntryChanged}},
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
// fails for an exported error type that carries a source position and has no
// Position method satisfying [Positioned], so an error type added later cannot
// miss it. A type carries a position when it has a field of type Pos or Token —
// cobol.Pos or cobol.Token outside the root package — directly, through a
// pointer, slice or array of one, or through an embedded struct of its own
// package that does. It also fails for an error type that carries no position
// and has a Position method anyway, which is what keeps "errors with no source
// position are unchanged" true.
func TestEveryPositionedErrorHasPosition(t *testing.T) {
	t.Parallel()

	types := scanErrorTypes(t, ".")

	var positioned []string
	for _, et := range types {
		if et.carriesPos {
			positioned = append(positioned, et.name)
			require.Truef(t, et.hasPosition, "%s carries a source position and has no Position method", et.name)
			require.Truef(t, et.positionOK, "%s has a Position method that does not satisfy Positioned: want Position() Pos", et.name)
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

// TestCarriesPos pins the scanner's reading of a struct, so the guard above
// cannot weaken unnoticed.
func TestCarriesPos(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		pkg  string
		src  string
		want bool
	}{
		{name: "root pos field", pkg: "cobol", src: "type E struct{ Pos Pos }", want: true},
		{name: "root token field", pkg: "cobol", src: "type E struct{ Actual Token }", want: true},
		{name: "root token slice", pkg: "cobol", src: "type E struct{ Tokens []Token }", want: true},
		{name: "root token pointer", pkg: "cobol", src: "type E struct{ Actual *Token }", want: true},
		{name: "qualified pos field", pkg: "copybook", src: "type E struct{ Pos cobol.Pos }", want: true},
		{name: "embedded base carrying pos", pkg: "copybook", src: "type base struct{ Pos cobol.Pos }\ntype E struct{ base }", want: true},
		{name: "local pos type outside the root", pkg: "picture", src: "type Pos int\ntype E struct{ Pos Pos }", want: false},
		{name: "byte offset", pkg: "picture", src: "type E struct{ Offset int }", want: false},
		{name: "qualified from another package", pkg: "codec", src: "type E struct{ Pos other.Pos }", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, err := goparser.ParseFile(token.NewFileSet(), "e.go", "package "+tc.pkg+"\n"+tc.src, goparser.SkipObjectResolution)
			require.NoError(t, err)
			structs := structTypes(f)
			require.Equal(t, tc.want, carriesPos(tc.pkg, structs["E"], structs, map[string]bool{}))
		})
	}
}

func TestIsPositionMethod(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		pkg  string
		src  string
		want bool
	}{
		{name: "root", pkg: "cobol", src: "func (e E) Position() Pos { return e.Pos }", want: true},
		{name: "qualified", pkg: "copybook", src: "func (e E) Position() cobol.Pos { return e.Pos }", want: true},
		{name: "wrong result type", pkg: "copybook", src: "func (e E) Position() int { return 0 }", want: false},
		{name: "two results", pkg: "cobol", src: "func (e E) Position() (Pos, bool) { return e.Pos, true }", want: false},
		{name: "takes a parameter", pkg: "cobol", src: "func (e E) Position(n int) Pos { return e.Pos }", want: false},
		{name: "unqualified outside the root", pkg: "copybook", src: "func (e E) Position() Pos { return e.Pos }", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, err := goparser.ParseFile(token.NewFileSet(), "e.go", "package "+tc.pkg+"\n"+tc.src, goparser.SkipObjectResolution)
			require.NoError(t, err)
			require.Equal(t, tc.want, isPositionMethod(tc.pkg, f.Decls[0].(*ast.FuncDecl)))
		})
	}
}

// errorType is one exported type, declared in a package of this module, that has
// an Error method.
type errorType struct {
	name        string // package-qualified outside the root package
	carriesPos  bool
	hasPosition bool // a method named Position exists
	positionOK  bool // and its signature is Position() Pos
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
	methods := map[string]map[string]*ast.FuncDecl{}
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
		for name, st := range structTypes(f) {
			structs[name] = st
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			recv := receiverName(fn.Recv.List[0].Type)
			if methods[recv] == nil {
				methods[recv] = map[string]*ast.FuncDecl{}
			}
			methods[recv][fn.Name.Name] = fn
		}
	}

	var out []errorType
	for name, st := range structs {
		if !ast.IsExported(name) || methods[name]["Error"] == nil {
			continue
		}
		qualified := name
		if pkg != "cobol" {
			qualified = pkg + "." + name
		}
		position := methods[name]["Position"]
		out = append(out, errorType{
			name:        qualified,
			carriesPos:  carriesPos(pkg, st, structs, map[string]bool{}),
			hasPosition: position != nil,
			positionOK:  position != nil && isPositionMethod(pkg, position),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

// structTypes returns every struct type a file declares, exported or not, so an
// embedded unexported base struct can be followed.
func structTypes(f *ast.File) map[string]*ast.StructType {
	out := map[string]*ast.StructType{}
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			if ts, ok := spec.(*ast.TypeSpec); ok {
				if st, ok := ts.Type.(*ast.StructType); ok {
					out[ts.Name.Name] = st
				}
			}
		}
	}
	return out
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

// rootTypeName returns the name of the root package type expr denotes — an
// unqualified identifier inside the root package, a cobol-qualified selector
// outside it — or "" when it denotes no root package type.
func rootTypeName(pkg string, expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.Ident:
		if pkg == "cobol" {
			return expr.Name
		}
	case *ast.SelectorExpr:
		if x, ok := expr.X.(*ast.Ident); ok && x.Name == "cobol" && pkg != "cobol" {
			return expr.Sel.Name
		}
	}
	return ""
}

// carriesPos reports whether a struct has a field of type Pos or Token — or a
// pointer, slice or array of one — or embeds a struct of its own package that
// does.
func carriesPos(pkg string, st *ast.StructType, structs map[string]*ast.StructType, seen map[string]bool) bool {
	for _, field := range st.Fields.List {
		typ := field.Type
		for unwrapped := false; !unwrapped; {
			switch t := typ.(type) {
			case *ast.StarExpr:
				typ = t.X
			case *ast.ArrayType:
				typ = t.Elt
			default:
				unwrapped = true
			}
		}
		if name := rootTypeName(pkg, typ); name == "Pos" || name == "Token" {
			return true
		}
		if len(field.Names) != 0 {
			continue
		}
		if id, ok := typ.(*ast.Ident); ok && !seen[id.Name] {
			seen[id.Name] = true
			if base, ok := structs[id.Name]; ok && carriesPos(pkg, base, structs, seen) {
				return true
			}
		}
	}
	return false
}

// isPositionMethod reports whether fn has the signature [Positioned] requires:
// no parameters and a single Pos result.
func isPositionMethod(pkg string, fn *ast.FuncDecl) bool {
	typ := fn.Type
	if typ.Params != nil && len(typ.Params.List) != 0 {
		return false
	}
	if typ.Results == nil || len(typ.Results.List) != 1 || len(typ.Results.List[0].Names) > 1 {
		return false
	}
	return rootTypeName(pkg, typ.Results.List[0].Type) == "Pos"
}

func TestPositionedErrorsWithoutPosition(t *testing.T) {
	t.Parallel()

	var p Positioned
	require.False(t, errors.As(UnexpectedEndOfTokensError{}, &p), "UnexpectedEndOfTokensError carries no position")
	require.False(t, errors.As(UnsupportedNodeError{}, &p), "UnsupportedNodeError carries no position")
}
