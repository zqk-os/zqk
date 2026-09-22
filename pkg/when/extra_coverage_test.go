// BLI-STARTER-COMMUNITY-033 / PRI-STARTER-COMMUNITY-033 coverage elevation
package when

import "testing"

func TestPredicatesAndPointers(t *testing.T) {
	t.Parallel()
	if !IsEmpty("") || IsEmpty("x") {
		t.Fatal("empty")
	}
	if !IsBlank(" \t") || IsBlank("a") {
		t.Fatal("blank")
	}
	var p *int
	if !IsNil(p) || IsNotNil(p) {
		t.Fatal("nil ptr")
	}
	n := 1
	if IsNil(&n) || !IsNotNil(&n) {
		t.Fatal("not nil")
	}
	var m map[string]int
	if !IsNilValue(m) || !IsNilOrEmpty(m) {
		t.Fatal("nil map")
	}
	if !IsNilOrEmpty("") || IsNilOrEmpty("z") {
		t.Fatal("string empty")
	}
	s := ""
	if !IsNilOrEmpty(&s) {
		t.Fatal("ptr empty string")
	}
	if !IsNewLine('\n') || !IsComma(',') || !IsColon(':') || !IsDash('-') || !IsHyphen('-') {
		t.Fatal("runes")
	}
	if !IsTilde('~') || !IsFwdSlash('/') || !IsBackSlash('\\') || !IsLeftCurly('{') || !IsRightCurly('}') || !IsDecimal('.') {
		t.Fatal("more runes")
	}
	if !IsChar('A') || !IsNum('9') {
		t.Fatal("char/num")
	}
	if PtrOr(p, 7) != 7 || PtrOr(&n, 0) != 1 {
		t.Fatal("ptror")
	}
	if PtrOrElse(p, func() int { return 8 }) != 8 {
		t.Fatal("ptrelse")
	}
	if PtrOrElse[int](nil, nil) != 0 {
		t.Fatal("ptrelse zero")
	}
	if *Ptr(3) != 3 {
		t.Fatal("ptr")
	}
}

func TestChainExecuteSignalsAndGeneric(t *testing.T) {
	t.Parallel()
	ran := false
	sig := When(func() bool { return true }).Then(func() { ran = true }).AndReturn().Execute()
	if !ran || sig != FlowReturn {
		t.Fatalf("return %v %v", ran, sig)
	}
	sig = When(func() bool { return false }).Then(func() {}).OrElse(func() {}).AndContinue().Execute()
	if sig != FlowContinue {
		t.Fatalf("continue %v", sig)
	}
	sig = When(func() bool { return false }).Then(func() {}).OrElse(func() {}).AndBreak().Execute()
	if sig != FlowBreak {
		t.Fatalf("break %v", sig)
	}
	WhenNilOrEmpty("").Then(func() { ran = true }).Run()
	WhenNotNilOrEmpty("x").ThenDo(func() { ran = true }).Run()
	WhenBlank(" ").Then(func() { ran = true }).Run()
	WhenNotBlank("z").Then(func() { ran = true }).Run()
	got := Result[int]().When(func() bool { return false }).Then(func() int { return 1 }).
		OrElseWhen(func() bool { return true }).Then(func() int { return 2 }).Run()
	if got != 2 {
		t.Fatalf("generic %d", got)
	}
	got = Result[int]().When(func() bool { return false }).Then(func() int { return 1 }).OrElse(func() int { return 9 }).Run()
	if got != 9 {
		t.Fatalf("generic else %d", got)
	}
}

func TestTry_ClearErrAndVal(t *testing.T) {
	t.Parallel()
	r := Try("a", errSentinel{}).ClearErr()
	if r.Err() != nil || r.Val() != "a" {
		t.Fatalf("%v %q", r.Err(), r.Val())
	}
}

type errSentinel struct{}

func (errSentinel) Error() string { return "e" }
