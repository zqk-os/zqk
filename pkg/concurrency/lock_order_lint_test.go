package concurrency_test

import (
	"bytes"
	"fmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLockOrderLint_CatchesKnownInvertedPair(t *testing.T) {
	tmpDir := t.TempDir()

	badCode := `
package dummy
import "sync"
func inverted() {
	var mu1, mu2 sync.Mutex
	mu1.Lock()
	mu2.Lock()
	mu2.Unlock()
	mu1.Unlock()
	
	mu2.Lock()
	mu1.Lock()
	mu1.Unlock()
	mu2.Unlock()
}
`
	err := fileutil.WriteFile(filepath.Join(tmpDir, "bad.go"), []byte(badCode), paths.FilePerm644) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}

	err = runLint(t, []string{tmpDir})
	if err == nil {
		t.Fatal("Expected linter to fail on known inverted pair, but it passed.")
	}
	if !strings.Contains(err.Error(), "INVERSION DETECTED") {
		t.Fatalf("Expected linter to report INVERSION DETECTED, got: %s", err.Error())
	}
}

func TestLockOrderLint_Project(t *testing.T) {
	err := runLint(t, []string{"../scheduler", "../storage"})
	if err != nil {
		t.Fatal(err)
	}
}

func runLint(t *testing.T, dirs []string) error {
	fset := token.NewFileSet()
	orderings := make(map[string]map[string]string)
	inversionsFound := false
	var errOut strings.Builder

	for _, dir := range dirs {
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error { //nolint:gosec
			if err != nil {
				return err
			}
			if !info.IsDir() && filepath.Ext(path) == ".go" {
				f, err := parser.ParseFile(fset, path, nil, 0)
				if err != nil {
					return nil
				}

				ast.Inspect(f, func(n ast.Node) bool {
					switch fn := n.(type) {
					case *ast.FuncDecl:
						var walkBlock func(stmts []ast.Stmt)
						walkBlock = func(stmts []ast.Stmt) {
							var held []string
							for _, stmt := range stmts {
								ast.Inspect(stmt, func(nn ast.Node) bool {
									if call, ok := nn.(*ast.CallExpr); ok {
										if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
											if sel.Sel.Name == "Lock" || sel.Sel.Name == "RLock" || sel.Sel.Name == "Unlock" || sel.Sel.Name == "RUnlock" {
												var buf bytes.Buffer
												printer.Fprint(&buf, fset, sel.X) //nolint:gosec
												name := buf.String()

												if sel.Sel.Name == "Lock" || sel.Sel.Name == "RLock" {
													for _, h := range held {
														if h != name {
															if orderings[name] != nil && orderings[name][h] != "" {
																msg := fmt.Sprintf("INVERSION DETECTED in %s:\n  Found %s -> %s\n  But previously found %s -> %s at %s\n", path, h, name, name, h, orderings[name][h])
																errOut.WriteString(msg)
																inversionsFound = true
															} else {
																if orderings[h] == nil {
																	orderings[h] = make(map[string]string)
																}
																orderings[h][name] = fmt.Sprintf("%s (func %s)", path, fn.Name)
															}
														}
													}
													held = append(held, name)
												} else {
													for i := len(held) - 1; i >= 0; i-- {
														if held[i] == name {
															held = append(held[:i], held[i+1:]...)
															break
														}
													}
												}
											}
										}
									}
									return true
								})
							}
						}
						if fn.Body != nil {
							walkBlock(fn.Body.List)
						}
					}
					return true
				})
			}
			return nil
		})
	}

	if inversionsFound {
		return fmt.Errorf("%s", errOut.String())
	}
	return nil
}
