package app

import (
	"os"
	"testing"
)

func TestExecute_Version(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"zqk", "version"}
	Execute()
}

func TestExecute_Help(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"zqk", "--help"}
	Execute()
}

func TestExecute_ZgrepPrefix(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"zgrep", "--help"}
	Execute()
}
