package generic

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/vedansh-5/graphcontext/pkg/lang"
)

// summarize renders an IR as sorted, readable lines for assertions.
func summarize(ir *lang.FileIR) string {
	var b strings.Builder
	for _, n := range ir.Nodes {
		flags := ""
		if n.IsTest {
			flags += " test"
		}
		if n.IsEntrypoint {
			flags += " main"
		}
		fmt.Fprintf(&b, "node %s %s %s%s\n", n.Kind, n.ID, n.Visibility, flags)
	}
	for _, r := range ir.Refs {
		from := r.FromID[strings.Index(r.FromID, ":")+1:]
		if r.Receiver != "" {
			fmt.Fprintf(&b, "ref %s %s -> %s.%s\n", r.Kind, from, r.Receiver, r.Name)
		} else {
			fmt.Fprintf(&b, "ref %s %s -> %s\n", r.Kind, from, r.Name)
		}
	}
	return b.String()
}

func parse(t *testing.T, path, src string) *lang.FileIR {
	t.Helper()
	p, ok := lang.For(path)
	if !ok {
		t.Fatalf("no plugin for %s", path)
	}
	ir, err := p.Parse(path, []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return ir
}

func assertIR(t *testing.T, ir *lang.FileIR, want string) {
	t.Helper()
	// Order within a line is not what these tests are about.
	sorted := func(s string) string {
		lines := strings.Split(strings.TrimSpace(s), "\n")
		sort.Strings(lines)
		return strings.Join(lines, "\n")
	}
	got := sorted(summarize(ir))
	want = sorted(want)
	if got != want {
		t.Errorf("IR mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
