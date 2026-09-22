// BLI-STARTER-COMMUNITY-038 / PRI-STARTER-COMMUNITY-038 coverage elevation
package functional

import (
	"context"
	"errors"
	"testing"
)

func TestExtraResultMapsAndApply(t *testing.T) {
	ok := Ok(7)
	if !ok.IsOk() || ok.IsErr() || ok.Unwrap() != 7 || ok.UnwrapOr(0) != 7 {
		t.Fatal("ok result")
	}
	v, err := ok.Value()
	if err != nil || v != 7 {
		t.Fatal(err)
	}
	fail := Err[int](errors.New("boom"))
	if !fail.IsErr() || fail.UnwrapOr(3) != 3 {
		t.Fatal("err result")
	}
	if fail.UnwrapOrElse(func(error) int { return 9 }) != 9 {
		t.Fatal("unwrap or else")
	}
	from := From(1, errors.New("x"))
	if from.IsOk() {
		t.Fatal("from err")
	}

	mapped := Map(ok, func(n int) int { return n + 1 })
	if mapped.Unwrap() != 8 {
		t.Fatal(mapped)
	}
	if Map(fail, func(n int) int { return n }).IsOk() {
		t.Fatal("map err")
	}
	if MapErr(ok, func(error) error { return errors.New("no") }).IsErr() {
		t.Fatal("maperr ok")
	}
	if !MapErr(fail, func(error) error { return errors.New("wrapped") }).IsErr() {
		t.Fatal("maperr fail")
	}
	if AndThen(ok, func(n int) Result[int] { return Ok(n * 2) }).Unwrap() != 14 {
		t.Fatal("andthen")
	}
	if AndThen(fail, func(n int) Result[int] { return Ok(n) }).IsOk() {
		t.Fatal("andthen err")
	}
	if OrElse(ok, Ok(0)).Unwrap() != 7 {
		t.Fatal("orelse ok")
	}
	if OrElse(fail, Ok(4)).Unwrap() != 4 {
		t.Fatal("orelse fail")
	}
	if OrElseGet(ok, func(error) Result[int] { return Ok(0) }).Unwrap() != 7 {
		t.Fatal("orelseget ok")
	}
	if OrElseGet(fail, func(error) Result[int] { return Ok(5) }).Unwrap() != 5 {
		t.Fatal("orelseget fail")
	}

	ctx := context.Background()
	applied := Apply(ctx, 2, func(n int) (int, error) { return n + 1, nil }, WithoutMetrics(), WithOperationType("add"))
	if applied.Unwrap() != 3 {
		t.Fatal(applied)
	}
	appliedErr := Apply(ctx, 2, func(int) (int, error) { return 0, errors.New("no") }, WithoutMetrics())
	if appliedErr.IsOk() {
		t.Fatal("apply err")
	}
	orElse := ApplyOrElse(ctx, 1, func(int) (int, error) { return 0, errors.New("no") }, func(error) (int, error) { return 8, nil }, WithoutMetrics())
	if orElse.Unwrap() != 8 {
		t.Fatal(orElse)
	}
	okChain := ApplyAndThen(ctx, 1, func(n int) (int, error) { return n + 1, nil }, func(n int) (int, error) { return n + 1, nil }, WithoutMetrics())
	if okChain.Unwrap() != 3 {
		t.Fatal(okChain)
	}
	errChain := ApplyAndThen(ctx, 1, func(int) (int, error) { return 0, errors.New("no") }, func(int) (int, error) { return 0, nil }, WithoutMetrics())
	if errChain.IsOk() {
		t.Fatal("applyandthen err")
	}
	if err := Do(ctx, 1, func(int) error { return nil }, WithoutMetrics()); err != nil {
		t.Fatal(err)
	}
	if err := DoOrElse(ctx, 1, func(int) error { return errors.New("no") }, func(error) error { return nil }, WithoutMetrics()); err != nil {
		t.Fatal(err)
	}
	if err := DoOrElse(ctx, 1, func(int) error { return nil }, func(error) error { return errors.New("x") }, WithoutMetrics()); err != nil {
		t.Fatal(err)
	}
	if err := DoAndThen(ctx, 1, func(n int) (int, error) { return n, nil }, func(int) error { return nil }, WithoutMetrics()); err != nil {
		t.Fatal(err)
	}
	if err := DoAndThen(ctx, 1, func(int) (int, error) { return 0, errors.New("no") }, func(int) error { return nil }, WithoutMetrics()); err == nil {
		t.Fatal("doandthen")
	}
	if Get(ctx, func() (int, error) { return 2, nil }, WithoutMetrics()).Unwrap() != 2 {
		t.Fatal("get")
	}
	if GetOrElse(ctx, func() (int, error) { return 0, errors.New("no") }, 11, WithoutMetrics()) != 11 {
		t.Fatal("getorelse")
	}
	if GetOrElseGet(ctx, func() (int, error) { return 0, errors.New("no") }, func(error) (int, error) { return 12, nil }, WithoutMetrics()).Unwrap() != 12 {
		t.Fatal("getorelseget")
	}
	if GetOrElseGet(ctx, func() (int, error) { return 4, nil }, func(error) (int, error) { return 0, nil }, WithoutMetrics()).Unwrap() != 4 {
		t.Fatal("getorelseget ok")
	}

	WithCoordinator(nil)(defaultApplyConfig())

	if MapGetPtr(map[string]*int(nil), "a") != nil || MapHasPtr(map[string]*int(nil), "a") {
		t.Fatal("nil ptr map")
	}
	n := 3
	m := map[string]*int{"a": &n}
	if MapGetPtr(m, "a") == nil || !MapHasPtr(m, "a") {
		t.Fatal("ptr map")
	}
	if MapGetOrDefault(map[string]int(nil), "a", 1) != 1 || MapGetOrDefault(map[string]int{"a": 2}, "a", 1) != 2 {
		t.Fatal("get or default")
	}
	if MapGetOrDefault(map[string]int{"a": 2}, "b", 1) != 1 {
		t.Fatal("missing default")
	}
	if MapKeys(map[string]int(nil)) != nil || len(MapKeys(map[string]int{"a": 1})) != 1 {
		t.Fatal("keys")
	}
	if MapValues(map[string]int(nil)) != nil || len(MapValues(map[string]int{"a": 1})) != 1 {
		t.Fatal("values")
	}
	When(func() bool { return true })
	defer func() {
		if recover() == nil {
			t.Fatal("expected unwrap panic")
		}
	}()
	fail.Unwrap()
}
