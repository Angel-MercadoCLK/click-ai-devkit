// This file hosts a second repo-wide guardrail, modeled on lint_test.go's wiring and control-test
// discipline: it guards the snapshot-drift-policy omission failure mode (teardown-rollback-hardening
// design item 2), the same failure class as the un-exhaustive StepActionKind switches — a forgotten
// classification silently produces wrong runtime behavior instead of a build error.
//
// installer.Step.Snapshot is typed as []installer.SnapshotDecl, and every production element must be
// built through the snapshot(path, policy) constructor with both arguments supplied. A composite
// literal element that is anything else (a bare struct literal, a call with the wrong arity, etc.)
// bypasses the compile-time guarantee the constructor exists to provide, so this detector re-asserts
// it via static analysis across the whole production tree — a second, independent layer beyond the
// type system, exactly as lint_test.go's regex is a second layer beyond code review.
package crossplatformlint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// snapshotDeclViolations inspects a parsed file for `Snapshot:` composite-literal fields whose
// elements are not calls to a function named "snapshot" with exactly two arguments, and returns one
// human-readable "file:line: message" string per offending element. It has no knowledge of whether
// "snapshot" resolves to installer's real constructor — that binding is enforced by the Go compiler
// once the real call shape is used; this only re-asserts the SHAPE is present at every call site, so
// it works equally well against real source files and small in-memory fixtures (used by the control
// test below), which never import the installer package at all.
func snapshotDeclViolations(fset *token.FileSet, file *ast.File, relPath string) []string {
	var offenders []string

	ast.Inspect(file, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Snapshot" {
			return true
		}
		elems, ok := kv.Value.(*ast.CompositeLit)
		if !ok {
			// Snapshot: someVariable or Snapshot: nil — not a literal we can inspect element-by-element.
			// Neither is the pattern being guarded against (a literal with a malformed element), so this
			// is deliberately not flagged; it would need its own, separate analysis to chase.
			return true
		}
		for _, elt := range elems.Elts {
			call, ok := elt.(*ast.CallExpr)
			if !ok {
				offenders = append(offenders, violation(fset, relPath, elt.Pos(), "Snapshot element is not a snapshot(path, policy) call"))
				continue
			}
			fn, ok := call.Fun.(*ast.Ident)
			if !ok || fn.Name != "snapshot" {
				offenders = append(offenders, violation(fset, relPath, elt.Pos(), "Snapshot element does not call the snapshot(...) constructor"))
				continue
			}
			if len(call.Args) != 2 {
				offenders = append(offenders, violation(fset, relPath, elt.Pos(), "snapshot(...) call does not have exactly two arguments (path, policy)"))
			}
		}
		return true
	})

	return offenders
}

func violation(fset *token.FileSet, relPath string, pos token.Pos, msg string) string {
	line := fset.Position(pos).Line
	return relPath + ":" + strconv.Itoa(line) + ": " + msg
}

// TestNoUnclassifiedSnapshotDeclarations walks every production (non-_test.go) *.go file in the
// module and fails if any Snapshot: literal element skips the snapshot(path, policy) constructor —
// preventing a new StepActionKind-adjacent snapshot path from silently landing without an explicit
// drift-policy classification.
func TestNoUnclassifiedSnapshotDeclarations(t *testing.T) {
	root := moduleRoot(t)
	var offenders []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		offenders = append(offenders, snapshotDeclViolations(fset, file, rel)...)
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module for production Go files failed: %v", err)
	}

	if len(offenders) > 0 {
		t.Fatalf("Snapshot: declarations must be built via snapshot(path, policy) with an explicit "+
			"DriftPolicy — a bare literal or wrong-arity call silently omits the classification "+
			"(teardown-rollback-hardening design item 2): %s", strings.Join(offenders, "; "))
	}
}

// TestSnapshotPolicyDetector_Controls is the detector's own unit test: it must catch the anti-pattern
// and must NOT flag the accepted form, so this guardrail can't silently rot into a no-op — mirroring
// TestDriveLetterDetector_Controls's discipline for the sibling guardrail in this package. The
// fixtures are small, self-contained, syntactically valid Go source parsed directly from a string —
// they never import package installer, which is exactly why the detector above matches on the call
// SHAPE (an identifier named "snapshot" with two arguments) rather than resolving the identifier
// against any particular package.
func TestSnapshotPolicyDetector_Controls(t *testing.T) {
	const badSource = `package fixture

type Step struct {
	Snapshot []SnapshotDecl
}

var steps = []Step{
	{Snapshot: []SnapshotDecl{{Path: "x", Policy: "whole-file-veto"}}},
}
`
	const badAritySource = `package fixture

type Step struct {
	Snapshot []SnapshotDecl
}

var steps = []Step{
	{Snapshot: []SnapshotDecl{snapshot("x")}},
}
`
	const goodSource = `package fixture

type Step struct {
	Snapshot []SnapshotDecl
}

var steps = []Step{
	{Snapshot: []SnapshotDecl{snapshot(cfg.ClaudeMDPath(), DriftPolicyManagedContentVeto), snapshot(cfg.ModelsPath(), DriftPolicyWholeFileVeto)}},
}
`

	for name, src := range map[string]string{"bare struct literal": badSource, "wrong-arity call": badAritySource} {
		t.Run("flags "+name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "fixture.go", src, 0)
			if err != nil {
				t.Fatalf("parser.ParseFile(%s fixture) error = %v", name, err)
			}
			if got := snapshotDeclViolations(fset, file, "fixture.go"); len(got) == 0 {
				t.Fatalf("snapshotDeclViolations() = %v, want at least one violation for %s", got, name)
			}
		})
	}

	t.Run("accepts fully-policied declarations", func(t *testing.T) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "fixture.go", goodSource, 0)
		if err != nil {
			t.Fatalf("parser.ParseFile(good fixture) error = %v", err)
		}
		if got := snapshotDeclViolations(fset, file, "fixture.go"); len(got) != 0 {
			t.Fatalf("snapshotDeclViolations() = %v, want no violations for a fully-policied declaration", got)
		}
	})
}
