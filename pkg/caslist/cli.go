package caslist

import (
	"errors"
	"os"
	"strings"
)

var errNotFound = errors.New("object not found")

// Try runs community inventory against the per-kind CAS index when argv is a
// simple object list/get/count. Returns handled=false so the caller can exec
// the full CLI for every other command or for flags this path does not own.
func Try(args []string) (handled bool, code int) {
	req, ok := parseInventory(args)
	if !ok {
		return false, 0
	}
	root := resolveProjectRoot(req.projectRoot, ".")
	if root == "" {
		writeStderr("caslist: project root not found (set ZCOM_PROJECT_ROOT)")
		return true, 1
	}
	proc := processDir(root)
	if st, err := os.Stat(proc); err != nil || !st.IsDir() {
		writeStderr("caslist: missing " + processDirName)
		return true, 1
	}
	if err := runInventory(req, proc); err != nil {
		if !errors.Is(err, errNotFound) {
			writeStderr("caslist: " + err.Error())
		}
		return true, 1
	}
	return true, 0
}

type inventoryReq struct {
	verb        string
	kind        string
	id          string
	format      string
	projectRoot string
}

func parseInventory(args []string) (inventoryReq, bool) {
	var req inventoryReq
	var positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") {
			name, value, hasVal, consumed, known := parseFlag(args, i)
			if !known {
				return inventoryReq{}, false
			}
			if consumed {
				i++
			}
			if !applyFlag(&req, name, value, hasVal) {
				return inventoryReq{}, false
			}
			continue
		}
		positionals = append(positionals, a)
	}
	if len(positionals) < 2 || positionals[0] != cmdObject {
		return inventoryReq{}, false
	}
	switch positionals[1] {
	case verbList, "ls", "find":
		req.verb = verbList
		if len(positionals) > 2 {
			req.kind = positionals[2]
		}
		if len(positionals) > 3 {
			return inventoryReq{}, false
		}
		if req.format == "" {
			req.format = defaultListFormat
		}
	case verbGet:
		req.verb = verbGet
		if len(positionals) != 3 {
			return inventoryReq{}, false
		}
		req.id = positionals[2]
		if req.format == "" {
			req.format = defaultGetFormat
		}
	case verbCount:
		req.verb = verbCount
		if len(positionals) > 2 {
			req.kind = positionals[2]
		}
		if len(positionals) > 3 {
			return inventoryReq{}, false
		}
		if req.format == "" {
			req.format = defaultListFormat
		}
	default:
		return inventoryReq{}, false
	}
	return req, true
}

func parseFlag(args []string, i int) (name, value string, hasVal, consumed, known bool) {
	a := args[i]
	if a == "-h" || a == "--help" || a == "--version" || a == "-v" {
		return "", "", false, false, false
	}
	if strings.HasPrefix(a, "--format=") {
		return "format", strings.TrimPrefix(a, "--format="), true, false, true
	}
	if strings.HasPrefix(a, "--project-root=") {
		return "project-root", strings.TrimPrefix(a, "--project-root="), true, false, true
	}
	if a == "--format" || a == "--project-root" {
		if i+1 >= len(args) {
			return "", "", false, false, false
		}
		return strings.TrimPrefix(a, "--"), args[i+1], true, true, true
	}
	return "", "", false, false, false
}

func applyFlag(req *inventoryReq, name, value string, hasVal bool) bool {
	if !hasVal || value == "" {
		return false
	}
	switch name {
	case "format":
		switch value {
		case formatJSON, formatYAML, formatTable:
			req.format = value
			return true
		default:
			return false
		}
	case "project-root":
		req.projectRoot = value
		return true
	default:
		return false
	}
}

func runInventory(req inventoryReq, proc string) error {
	switch req.verb {
	case verbList:
		return runList(req, proc)
	case verbGet:
		return runGet(req, proc)
	case verbCount:
		return runCount(req, proc)
	default:
		return nil
	}
}

func skipOnAllKindsList(kind string) bool {
	// Untyped `object list` is the first-run scoreboard. Opening every
	// doc_entry / object_spec YAML is the old 2s+ tax in a different shape.
	// Named `object list doc_entry` still uses the kind index.
	return kind == kindDocEntry || kind == kindObjectSpec
}

func runList(req inventoryReq, proc string) error {
	if req.kind != "" {
		dir, ok := findKindDir(proc, req.kind)
		if !ok {
			return writeList(req.format, nil)
		}
		objs, err := listKind(dir, req.kind)
		if err != nil {
			return err
		}
		return writeList(req.format, objs)
	}
	var all []map[string]any
	err := eachIndexedKind(proc, func(kind, dir string) error {
		if skipOnAllKindsList(kind) {
			return nil
		}
		objs, err := listKind(dir, kind)
		if err != nil {
			return err
		}
		all = append(all, objs...)
		return nil
	})
	if err != nil {
		return err
	}
	return writeList(req.format, all)
}

func runGet(req inventoryReq, proc string) error {
	var found map[string]any
	err := eachIndexedKind(proc, func(kind, dir string) error {
		if found != nil {
			return nil
		}
		obj, err := getByID(dir, kind, req.id)
		if err != nil {
			return err
		}
		if obj != nil {
			found = obj
		}
		return nil
	})
	if err != nil {
		return err
	}
	if found == nil {
		writeStderr("caslist: object not found: " + req.id)
		return errNotFound
	}
	return writeGet(req.format, found)
}

func runCount(req inventoryReq, proc string) error {
	if req.kind != "" {
		dir, ok := findKindDir(proc, req.kind)
		if !ok {
			return writeCount(req.format, req.kind, nil, nil)
		}
		objs, err := listKind(dir, req.kind)
		if err != nil {
			return err
		}
		return writeCount(req.format, req.kind, objs, nil)
	}
	byKind := map[string]int{}
	err := eachIndexedKind(proc, func(kind, dir string) error {
		objs, err := listKind(dir, kind)
		if err != nil {
			return err
		}
		byKind[kind] = len(objs)
		return nil
	})
	if err != nil {
		return err
	}
	return writeCount(req.format, "", nil, byKind)
}
