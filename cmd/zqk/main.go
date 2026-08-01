package main

import (
	"fmt"
	"os"
)

var (
	bin     = "zqk"
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("Executable: %s\nVersion: %s\nBuild datetime: %s\nCommit hash: %s\n", bin, version, date, commit)
		return
	}
	fmt.Println("temporary")
}
