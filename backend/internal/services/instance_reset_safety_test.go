package services

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// This test protects the core data-safety contract of Reset. A reset may
// rebuild ephemeral compute, but it must never enter instance deletion or PVC
// cleanup paths. Runtime behavior tests cover successful recreation; this AST
// guard prevents a future refactor from silently widening the operation.
func TestResetDoesNotInvokePersistentResourceDeletion(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "instance_service.go", nil, 0)
	if err != nil {
		t.Fatalf("parse instance_service.go: %v", err)
	}
	var resetBody *ast.BlockStmt
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Recv != nil && function.Name.Name == "Reset" {
			resetBody = function.Body
			break
		}
	}
	if resetBody == nil {
		t.Fatal("Reset method not found")
	}
	forbidden := map[string]bool{
		"Delete": true, "DeletePVC": true, "DeletePVCByName": true,
		"deleteInstancePVC": true, "CleanupInstance": true,
	}
	pvcChecks := 0
	ast.Inspect(resetBody, func(node ast.Node) bool {
		switch expression := node.(type) {
		case *ast.SelectorExpr:
			if forbidden[expression.Sel.Name] {
				t.Errorf("Reset must not call persistent cleanup method %s", expression.Sel.Name)
			}
			if expression.Sel.Name == "GetPVCByName" {
				pvcChecks++
			}
		case *ast.Ident:
			if expression.Name == "deleteInstancePVC" || expression.Name == "CleanupInstance" {
				t.Errorf("Reset must not enter persistent cleanup path %s", expression.Name)
			}
		}
		return true
	})
	if pvcChecks < 3 {
		t.Fatalf("Reset performs %d exact PVC checks, want preflight, pre-start and post-start checks", pvcChecks)
	}
}
