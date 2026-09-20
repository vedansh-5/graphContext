package diff

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vedansh-5/graphcontext/pkg/store"
)

const sampleDiff = `diff --git a/auth/service.go b/auth/service.go
index abc1234..def5678 100644
--- a/auth/service.go
+++ b/auth/service.go
@@ -5,6 +5,8 @@ package auth
 func Login() bool {
-	return false
+	validateUser()
+	return true
 }
 
 func Logout() {}
diff --git a/auth/new_feature.go b/auth/new_feature.go
new file mode 100644
index 0000000..1234567
--- /dev/null
+++ b/auth/new_feature.go
@@ -0,0 +1,5 @@
+package auth
+
+func NewToken() string {
+	return "token"
+}
diff --git a/auth/legacy.go b/auth/legacy.go
deleted file mode 100644
index 1234567..0000000
--- a/auth/legacy.go
+++ /dev/null
@@ -1,5 +0,0 @@
-package auth
-
-func OldFunc() {}
`

func TestParseUnifiedDiff(t *testing.T) {
	diffs, err := ParseUnifiedDiff(strings.NewReader(sampleDiff))
	if err != nil {
		t.Fatalf("ParseUnifiedDiff failed: %v", err)
	}

	if len(diffs) != 3 {
		t.Fatalf("expected 3 file diffs, got %d", len(diffs))
	}

	d1 := diffs[0]
	if d1.NewPath != "auth/service.go" {
		t.Errorf("expected path auth/service.go, got %s", d1.NewPath)
	}
	if len(d1.AddedLines) != 2 || d1.AddedLines[0] != 6 || d1.AddedLines[1] != 7 {
		t.Errorf("expected added lines [6 7], got %v", d1.AddedLines)
	}
	if len(d1.DeletedLines) != 1 || d1.DeletedLines[0] != 6 {
		t.Errorf("expected deleted lines [6], got %v", d1.DeletedLines)
	}
	if len(d1.ModifiedRanges) != 1 || d1.ModifiedRanges[0].Start != 6 || d1.ModifiedRanges[0].Count != 2 {
		t.Errorf("expected 1 modified range {6, 2}, got %v", d1.ModifiedRanges)
	}

	d2 := diffs[1]
	if !d2.IsNew {
		t.Errorf("expected d2 to be marked as new file")
	}
	if d2.NewPath != "auth/new_feature.go" {
		t.Errorf("expected path auth/new_feature.go, got %s", d2.NewPath)
	}
	if len(d2.AddedLines) != 5 {
		t.Errorf("expected 5 added lines, got %d", len(d2.AddedLines))
	}

	d3 := diffs[2]
	if !d3.IsDeleted {
		t.Errorf("expected d3 to be marked as deleted file")
	}
	if d3.OldPath != "auth/legacy.go" {
		t.Errorf("expected path auth/legacy.go, got %s", d3.OldPath)
	}
}

func TestMapDiffToSymbols(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer s.Close()

	b := store.NewBatch()
	b.AddNode(store.Node{
		ID:        "auth/service.go:Login",
		Name:      "Login",
		FilePath:  "auth/service.go",
		StartLine: 5,
		EndLine:   10,
		Kind:      store.KindFunction,
	})
	b.AddNode(store.Node{
		ID:        "auth/service.go:Logout",
		Name:      "Logout",
		FilePath:  "auth/service.go",
		StartLine: 12,
		EndLine:   15,
		Kind:      store.KindFunction,
	})
	b.AddNode(store.Node{
		ID:        "auth/new_feature.go:NewToken",
		Name:      "NewToken",
		FilePath:  "auth/new_feature.go",
		StartLine: 3,
		EndLine:   5,
		Kind:      store.KindFunction,
	})
	b.AddNode(store.Node{
		ID:        "auth/legacy.go:OldFunc",
		Name:      "OldFunc",
		FilePath:  "auth/legacy.go",
		StartLine: 3,
		EndLine:   5,
		Kind:      store.KindFunction,
	})

	if err := s.Commit(b); err != nil {
		t.Fatalf("commit failed: %v", err)
	}

	diffs, err := ParseUnifiedDiff(strings.NewReader(sampleDiff))
	if err != nil {
		t.Fatalf("ParseUnifiedDiff failed: %v", err)
	}

	changed, err := MapDiffToSymbols(diffs, s)
	if err != nil {
		t.Fatalf("MapDiffToSymbols failed: %v", err)
	}

	if len(changed) != 3 {
		t.Fatalf("expected 3 changed symbols, got %d", len(changed))
	}

	// changed symbols are sorted by ID
	oldSym := changed[0]
	if oldSym.Node.ID != "auth/legacy.go:OldFunc" {
		t.Errorf("expected OldFunc symbol at index 0, got %s", oldSym.Node.ID)
	}
	if oldSym.ChangeType != "deleted" {
		t.Errorf("expected OldFunc to be deleted, got %s", oldSym.ChangeType)
	}

	tokenSym := changed[1]
	if tokenSym.Node.ID != "auth/new_feature.go:NewToken" {
		t.Errorf("expected NewToken symbol at index 1, got %s", tokenSym.Node.ID)
	}
	if tokenSym.ChangeType != "added" {
		t.Errorf("expected NewToken to be added, got %s", tokenSym.ChangeType)
	}

	loginSym := changed[2]
	if loginSym.Node.ID != "auth/service.go:Login" {
		t.Errorf("expected Login symbol at index 2, got %s", loginSym.Node.ID)
	}
	if loginSym.ChangeType != "modified" {
		t.Errorf("expected Login to be modified, got %s", loginSym.ChangeType)
	}
}

func TestRunGitDiff(t *testing.T) {
	dir := t.TempDir()
	initCmd := exec.Command("git", "init")
	initCmd.Dir = dir
	if err := initCmd.Run(); err != nil {
		t.Skip("git not available or init failed")
	}

	exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()
	exec.Command("git", "-C", dir, "config", "user.email", "test@example.com").Run()

	testFile := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(testFile, []byte("line 1\nline 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "initial").Run()

	if err := os.WriteFile(testFile, []byte("line 1\nline 2 modified\nline 3 added\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	diffs, err := RunGitDiff(dir)
	if err != nil {
		t.Fatalf("RunGitDiff failed: %v", err)
	}

	if len(diffs) != 1 {
		t.Fatalf("expected 1 file diff, got %d", len(diffs))
	}
	if diffs[0].Path() != "hello.txt" {
		t.Errorf("expected hello.txt, got %s", diffs[0].Path())
	}
	if len(diffs[0].AddedLines) == 0 {
		t.Errorf("expected added lines in diff")
	}
}
