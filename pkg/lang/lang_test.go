package lang

import (
	"reflect"
	"testing"
)

type fakeLang struct {
	name string
	exts []string
}

func (f fakeLang) Name() string                              { return f.name }
func (f fakeLang) Extensions() []string                      { return f.exts }
func (f fakeLang) ModulePath(root, path string) string       { return path }
func (f fakeLang) Parse(p string, s []byte) (*FileIR, error) { return &FileIR{Path: p}, nil }

func TestRegistryLookupByExtension(t *testing.T) {
	reset()
	t.Cleanup(reset)

	Register(fakeLang{name: "go", exts: []string{".go"}})
	Register(fakeLang{name: "typescript", exts: []string{".ts", ".tsx"}})

	for path, want := range map[string]string{
		"a/b/main.go": "go",
		"src/App.tsx": "typescript",
		"src/util.TS": "typescript", // extension match is case-insensitive
	} {
		l, ok := For(path)
		if !ok {
			t.Fatalf("For(%q): no plugin", path)
		}
		if l.Name() != want {
			t.Errorf("For(%q) = %q, want %q", path, l.Name(), want)
		}
	}

	if _, ok := For("README.md"); ok {
		t.Error("unregistered extension should not resolve")
	}

	got := Extensions()
	want := []string{".go", ".ts", ".tsx"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Extensions() = %v, want %v", got, want)
	}
}

func TestDuplicateExtensionPanics(t *testing.T) {
	reset()
	t.Cleanup(reset)

	Register(fakeLang{name: "first", exts: []string{".go"}})
	defer func() {
		if recover() == nil {
			t.Error("registering a duplicate extension should panic")
		}
	}()
	Register(fakeLang{name: "second", exts: []string{".go"}})
}

func TestVarKeyIsScoped(t *testing.T) {
	// The same variable name in two functions must not collide.
	a := VarKey("f.go:Alpha", "repo")
	b := VarKey("f.go:Beta", "repo")
	if a == b {
		t.Fatal("VarKey must include the enclosing scope")
	}
	tf := NewTypeFacts()
	tf.Vars[a] = "UserRepo"
	tf.Vars[b] = "OrderRepo"
	if tf.Vars[a] == tf.Vars[b] {
		t.Error("scoped keys should hold distinct types")
	}
}
