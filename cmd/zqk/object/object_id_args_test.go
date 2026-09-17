package object

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func TestExpandObjectIDArgs(t *testing.T) {
	t.Parallel()

	t.Run("comma_positional", func(t *testing.T) {
		t.Parallel()
		got := expandObjectIDArgs(nil, []string{"ATK-1,ATK-2", "ATK-3"})
		want := []string{"ATK-1", "ATK-2", "ATK-3"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v want %#v", got, want)
		}
	})

	t.Run("ids_flag", func(t *testing.T) {
		t.Parallel()
		cmd := &cobra.Command{Use: "promote"}
		cmd.Flags().String("ids", "", "")
		_ = cmd.Flags().Set("ids", "ATK-4,ATK-5")
		got := expandObjectIDArgs(cmd, []string{"ATK-1"})
		want := []string{"ATK-1", "ATK-4", "ATK-5"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v want %#v", got, want)
		}
	})
}
