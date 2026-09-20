package when

import "testing"

const (
	testLabelFirstBranch  = "first branch"
	testLabelSecondBranch = "second branch"
	testLabelOrElseBranch = "OrElse branch"
	testLabelNilOrEmpty   = "WhenNilOrEmpty"
	testLabelNotNilEmpty  = "WhenNotNilOrEmpty"
	testLabelBlank        = "WhenBlank"
	testLabelNotBlank     = "WhenNotBlank"

	testRunFirst    = "first"
	testRunSecond   = "second"
	testRunElse     = "else"
	testRunEmpty    = "empty"
	testRunNonEmpty = "non-empty"
	testRunBlank    = "blank"
	testRunNotBlank = "not-blank"
	testEmptyString = ""
)

func assertRan(t *testing.T, got, want, label string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: ran = %q, want %q", label, got, want)
	}
}

func TestChain_Run(t *testing.T) {
	var ran string
	When(func() bool { return true }).Then(func() { ran = testRunFirst }).Run()
	assertRan(t, ran, testRunFirst, testLabelFirstBranch)

	ran = testEmptyString
	When(func() bool { return false }).Then(func() { ran = testRunFirst }).
		OrElseWhen(func() bool { return true }).Then(func() { ran = testRunSecond }).Run()
	assertRan(t, ran, testRunSecond, testLabelSecondBranch)

	ran = testEmptyString
	When(func() bool { return false }).Then(func() { ran = testRunFirst }).
		OrElseWhen(func() bool { return false }).Then(func() { ran = testRunSecond }).
		OrElse(func() { ran = testRunElse }).Run()
	assertRan(t, ran, testRunElse, testLabelOrElseBranch)
}

func TestChain_ConvenienceBuilders(t *testing.T) {
	var ran string
	WhenNilOrEmpty("").ThenDo(func() { ran = testRunEmpty }).Run()
	assertRan(t, ran, testRunEmpty, testLabelNilOrEmpty)

	ran = testEmptyString
	WhenNotNilOrEmpty("x").ThenDo(func() { ran = testRunNonEmpty }).Run()
	assertRan(t, ran, testRunNonEmpty, testLabelNotNilEmpty)

	ran = testEmptyString
	WhenBlank(" \t").ThenDo(func() { ran = testRunBlank }).Run()
	assertRan(t, ran, testRunBlank, testLabelBlank)

	ran = testEmptyString
	WhenNotBlank("value").ThenDo(func() { ran = testRunNotBlank }).Run()
	assertRan(t, ran, testRunNotBlank, testLabelNotBlank)
}

func TestChain_ExecuteFlowSignals(t *testing.T) {
	sig := WhenNilOrEmpty(testEmptyString).ThenDo(func() {}).AndReturn().Execute()
	if sig != FlowReturn {
		t.Fatalf("expected FlowReturn, got %v", sig)
	}

	sig = WhenNotBlank("x").ThenDo(func() {}).AndContinue().Execute()
	if sig != FlowContinue {
		t.Fatalf("expected FlowContinue, got %v", sig)
	}

	sig = WhenBlank(" ").ThenDo(func() {}).AndBreak().Execute()
	if sig != FlowBreak {
		t.Fatalf("expected FlowBreak, got %v", sig)
	}

	sig = WhenBlank("x").ThenDo(func() {}).AndReturn().Execute()
	if sig != FlowNone {
		t.Fatalf("expected FlowNone on non-match, got %v", sig)
	}
}
