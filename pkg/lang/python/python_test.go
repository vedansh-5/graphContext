package python

import (
	"testing"

	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

const fixture = `from auth.models import User, Session
from .helpers import *
import numpy as np

@app.route("/login")
def login(u: User) -> bool:
    """Authenticate a user."""
    return u.check()

def main():
    repo = UserRepo(None)
    repo.save(User())

class Base:
    def ping(self): pass

class UserRepo(Base):
    """Stores users."""

    def __init__(self, db: DB):
        self.db = db
        self.cache = Cache()

    def save(self, u: User):
        return self.db.write(u)

    def _internal(self):
        pass
`

func parse(t *testing.T, path, src string) *lang.FileIR {
	t.Helper()
	ir, err := Plugin{}.Parse(path, []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return ir
}

func TestDeclarations(t *testing.T) {
	ir := parse(t, "repo.py", fixture)
	got := map[string]store.NodeKind{}
	for _, n := range ir.Nodes {
		got[n.ID] = n.Kind
	}
	for id, kind := range map[string]store.NodeKind{
		"repo.py:login":             store.KindFunction,
		"repo.py:main":              store.KindFunction,
		"repo.py:Base":              store.KindClass,
		"repo.py:UserRepo":          store.KindClass,
		"repo.py:UserRepo.__init__": store.KindMethod,
		"repo.py:UserRepo.save":     store.KindMethod,
	} {
		if got[id] != kind {
			t.Errorf("node %s = %q, want %q", id, got[id], kind)
		}
	}
}

func TestImportsIncludingWildcard(t *testing.T) {
	ir := parse(t, "repo.py", fixture)
	if len(ir.Imports) != 3 {
		t.Fatalf("imports = %+v", ir.Imports)
	}
	if ir.Imports[0].Path != "auth.models" || len(ir.Imports[0].Names) != 2 {
		t.Errorf("from-import = %+v", ir.Imports[0])
	}
	if !ir.Imports[1].Wildcard {
		t.Errorf("wildcard import not detected: %+v", ir.Imports[1])
	}
	if ir.Imports[2].Path != "numpy" || ir.Imports[2].Alias != "np" {
		t.Errorf("aliased import = %+v", ir.Imports[2])
	}
}

// self carries the enclosing class's type, and __init__ assignments become
// field types. Together these make self.db.write() resolvable without inference.
func TestTypeFacts(t *testing.T) {
	ir := parse(t, "repo.py", fixture)

	if got := ir.Types.Vars[lang.VarKey("repo.py:UserRepo.save", "self")]; got != "UserRepo" {
		t.Errorf("self type = %q, want UserRepo", got)
	}
	if got := ir.Types.Fields["UserRepo.db"]; got != "DB" {
		t.Errorf("field from annotated param = %q, want DB", got)
	}
	if got := ir.Types.Fields["UserRepo.cache"]; got != "Cache" {
		t.Errorf("field from constructor = %q, want Cache", got)
	}
	if got := ir.Types.Vars[lang.VarKey("repo.py:login", "u")]; got != "User" {
		t.Errorf("annotated param = %q, want User", got)
	}
	if got := ir.Types.Vars[lang.VarKey("repo.py:main", "repo")]; got != "UserRepo" {
		t.Errorf("constructor local = %q, want UserRepo", got)
	}
}

func TestInheritanceRecordedAsFactAndRef(t *testing.T) {
	ir := parse(t, "repo.py", fixture)
	if got := ir.Types.Bases["UserRepo"]; len(got) != 1 || got[0] != "Base" {
		t.Errorf("bases = %v, want [Base]", got)
	}
	var found bool
	for _, r := range ir.Refs {
		if r.Kind == store.EdgeInherits && r.FromID == "repo.py:UserRepo" && r.Name == "Base" {
			found = true
		}
	}
	if !found {
		t.Error("missing inherits ref")
	}
}

func TestDecoratorMarksRoute(t *testing.T) {
	ir := parse(t, "repo.py", fixture)
	for _, n := range ir.Nodes {
		if n.ID == "repo.py:login" {
			if !n.IsEntrypoint || n.EntrypointKind != store.EntryRoute {
				t.Errorf("login should be a route entrypoint, got %q", n.EntrypointKind)
			}
			if n.Docstring != "Authenticate a user." {
				t.Errorf("docstring = %q", n.Docstring)
			}
		}
	}
}

func TestRefsCarryReceiver(t *testing.T) {
	ir := parse(t, "repo.py", fixture)
	type key struct{ from, recv, name string }
	got := map[key]bool{}
	for _, r := range ir.Refs {
		if r.Kind == store.EdgeCalls {
			got[key{r.FromID, r.Receiver, r.Name}] = true
		}
	}
	for _, want := range []key{
		{"repo.py:UserRepo.save", "self.db", "write"},
		{"repo.py:main", "repo", "save"},
		{"repo.py:login", "u", "check"},
	} {
		if !got[want] {
			t.Errorf("missing ref %+v", want)
		}
	}
}

func TestVisibilityAndTestDetection(t *testing.T) {
	ir := parse(t, "repo.py", fixture)
	for _, n := range ir.Nodes {
		if n.ID == "repo.py:UserRepo._internal" && n.Visibility != store.VisPrivate {
			t.Error("_internal should be private")
		}
	}
	tests := parse(t, "tests/test_auth.py", "def test_login():\n    pass\n\ndef helper():\n    pass\n")
	for _, n := range tests.Nodes {
		switch n.Name {
		case "test_login":
			if !n.IsTest {
				t.Error("test_login should be a test")
			}
		case "helper":
			if n.IsTest {
				t.Error("helper is not a test")
			}
		}
	}
}

func TestModulePath(t *testing.T) {
	for path, want := range map[string]string{
		"/r/auth/models.py":    "auth.models",
		"/r/auth/__init__.py":  "auth",
		"/r/pkg/sub/thing.pyi": "pkg.sub.thing",
	} {
		if got := (Plugin{}).ModulePath("/r", path); got != want {
			t.Errorf("ModulePath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestDeterministic(t *testing.T) {
	a := parse(t, "repo.py", fixture)
	for i := 0; i < 5; i++ {
		b := parse(t, "repo.py", fixture)
		if len(a.Nodes) != len(b.Nodes) || len(a.Refs) != len(b.Refs) {
			t.Fatal("IR size varies")
		}
		for j := range a.Refs {
			if a.Refs[j] != b.Refs[j] {
				t.Fatalf("ref order varies at %d", j)
			}
		}
	}
}
