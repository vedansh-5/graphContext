package golang

import (
	"testing"

	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

const fixture = `package main

import (
	"fmt"
	str "strings"
)

type DB struct{ dsn string }

func (d *DB) Write(u User) error { return nil }

type UserRepo struct {
	db  *DB
	log Logger
}

func (r *UserRepo) Save(u User) error {
	return r.db.Write(u)
}

type Store interface {
	Save(u User) error
	Close() error
}

func main() {
	repo := UserRepo{}
	repo.Save(User{})
	fmt.Println(str.ToUpper("x"))
}
`

func parse(t *testing.T, path, src string) *lang.FileIR {
	t.Helper()
	ir, err := Plugin{}.Parse(path, []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return ir
}

func TestParseDeclarations(t *testing.T) {
	ir := parse(t, "svc.go", fixture)

	want := map[string]store.NodeKind{
		"svc.go:DB":            store.KindClass,
		"svc.go:DB.Write":      store.KindMethod,
		"svc.go:UserRepo":      store.KindClass,
		"svc.go:UserRepo.Save": store.KindMethod,
		"svc.go:Store":         store.KindInterface,
		"svc.go:main":          store.KindFunction,
	}
	got := map[string]store.NodeKind{}
	for _, n := range ir.Nodes {
		got[n.ID] = n.Kind
	}
	for id, kind := range want {
		if got[id] != kind {
			t.Errorf("node %s = %q, want %q", id, got[id], kind)
		}
	}

	for _, n := range ir.Nodes {
		if n.ID == "svc.go:main" {
			if !n.IsEntrypoint || n.EntrypointKind != store.EntryMain {
				t.Errorf("main should be an entrypoint, got %+v", n.EntrypointKind)
			}
		}
		if n.ID == "svc.go:UserRepo.Save" && n.Visibility != store.VisExported {
			t.Errorf("Save should be exported")
		}
	}
}

func TestImports(t *testing.T) {
	ir := parse(t, "svc.go", fixture)
	if len(ir.Imports) != 2 {
		t.Fatalf("imports = %+v", ir.Imports)
	}
	if ir.Imports[0].Path != "fmt" {
		t.Errorf("import[0] = %q, want fmt", ir.Imports[0].Path)
	}
	if ir.Imports[1].Path != "strings" || ir.Imports[1].Alias != "str" {
		t.Errorf("aliased import = %+v", ir.Imports[1])
	}
}

// The whole point of the Go plugin: receiver and field types are visible, so
// "r.db.Write" is resolvable without any type inference.
func TestTypeFactsEnableMethodResolution(t *testing.T) {
	ir := parse(t, "svc.go", fixture)

	if got := ir.Types.Vars[lang.VarKey("svc.go:UserRepo.Save", "r")]; got != "UserRepo" {
		t.Errorf("receiver type = %q, want UserRepo", got)
	}
	if got := ir.Types.Fields["UserRepo.db"]; got != "DB" {
		t.Errorf("field type = %q, want DB", got)
	}
	if got := ir.Types.Vars[lang.VarKey("svc.go:main", "repo")]; got != "UserRepo" {
		t.Errorf("composite literal type = %q, want UserRepo", got)
	}
	if got := ir.Types.Vars[lang.VarKey("svc.go:UserRepo.Save", "u")]; got != "User" {
		t.Errorf("param type = %q, want User", got)
	}
}

// Interface method sets drive structural `implements` resolution in pass 2.
func TestInterfaceAndMethodSets(t *testing.T) {
	ir := parse(t, "svc.go", fixture)
	if got := ir.Types.Methods["Store"]; len(got) != 2 || got[0] != "Close" || got[1] != "Save" {
		t.Errorf("Store method set = %v, want [Close Save]", got)
	}
	if got := ir.Types.Methods["UserRepo"]; len(got) != 1 || got[0] != "Save" {
		t.Errorf("UserRepo method set = %v, want [Save]", got)
	}
}

func TestRefsCarryReceiverAndScope(t *testing.T) {
	ir := parse(t, "svc.go", fixture)

	type key struct{ from, recv, name string }
	got := map[key]bool{}
	for _, r := range ir.Refs {
		got[key{r.FromID, r.Receiver, r.Name}] = true
	}

	for _, want := range []key{
		{"svc.go:UserRepo.Save", "r.db", "Write"},
		{"svc.go:main", "repo", "Save"},
		{"svc.go:main", "fmt", "Println"},
		{"svc.go:main", "str", "ToUpper"},
	} {
		if !got[want] {
			t.Errorf("missing ref %+v; have %+v", want, ir.Refs)
		}
	}
}

func TestTestFileDetection(t *testing.T) {
	ir := parse(t, "svc_test.go", "package p\n\nfunc TestThing(t *T) {}\nfunc helper() {}\n")
	for _, n := range ir.Nodes {
		switch n.ID {
		case "svc_test.go:TestThing":
			if !n.IsTest {
				t.Error("TestThing should be marked as a test")
			}
		case "svc_test.go:helper":
			if n.IsTest {
				t.Error("helper is not a test function")
			}
		}
	}
}

func TestParseIsDeterministic(t *testing.T) {
	a := parse(t, "svc.go", fixture)
	for i := 0; i < 5; i++ {
		b := parse(t, "svc.go", fixture)
		if len(a.Nodes) != len(b.Nodes) || len(a.Refs) != len(b.Refs) {
			t.Fatal("IR size varies between parses")
		}
		for j := range a.Nodes {
			if a.Nodes[j].ID != b.Nodes[j].ID {
				t.Fatalf("node order varies: %s vs %s", a.Nodes[j].ID, b.Nodes[j].ID)
			}
		}
		for j := range a.Refs {
			if a.Refs[j] != b.Refs[j] {
				t.Fatalf("ref order varies at %d", j)
			}
		}
	}
}

func TestModulePathDropsFileName(t *testing.T) {
	if got := (Plugin{}).ModulePath("/repo", "/repo/pkg/store/query.go"); got != "pkg/store" {
		t.Errorf("ModulePath = %q, want pkg/store", got)
	}
}
