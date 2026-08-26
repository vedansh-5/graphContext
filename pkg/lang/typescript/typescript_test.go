package typescript

import (
	"reflect"
	"testing"

	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

const fixture = `import { User, Session } from './models';
import * as helpers from './helpers';
import DefaultClient from 'client';
export * from './types';

/**
 * Store interface documentation
 */
export interface Store {
  save(u: User): Promise<void>;
  close(): Promise<void>;
}

export class Base {
  ping(): void {}
}

/**
 * UserRepo manages users
 */
export class UserRepo extends Base implements Store {
  private db: DB;
  cache = new Cache();

  constructor(db: DB) {
    super();
    this.db = db;
  }

  async save(u: User): Promise<void> {
    return this.db.write(u);
  }

  private internal(): void {}
}

export const helperFunc = (val: string): boolean => {
  return val.length > 0;
};

export function main(): void {
  const repo = new UserRepo(null);
  repo.save(new User());
  helpers.doSomething();
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

func TestDeclarations(t *testing.T) {
	ir := parse(t, "repo.ts", fixture)
	got := map[string]store.NodeKind{}
	for _, n := range ir.Nodes {
		got[n.ID] = n.Kind
	}
	want := map[string]store.NodeKind{
		"repo.ts:Store":                store.KindInterface,
		"repo.ts:Base":                 store.KindClass,
		"repo.ts:Base.ping":            store.KindMethod,
		"repo.ts:UserRepo":             store.KindClass,
		"repo.ts:UserRepo.constructor": store.KindMethod,
		"repo.ts:UserRepo.save":        store.KindMethod,
		"repo.ts:UserRepo.internal":    store.KindMethod,
		"repo.ts:helperFunc":           store.KindFunction,
		"repo.ts:main":                 store.KindFunction,
	}
	for id, kind := range want {
		if got[id] != kind {
			t.Errorf("node %s = %q, want %q", id, got[id], kind)
		}
	}
}

func TestImports(t *testing.T) {
	ir := parse(t, "repo.ts", fixture)
	if len(ir.Imports) != 4 {
		t.Fatalf("imports len = %d, want 4 (%+v)", len(ir.Imports), ir.Imports)
	}
	if ir.Imports[0].Path != "./models" || len(ir.Imports[0].Names) != 2 {
		t.Errorf("named import = %+v", ir.Imports[0])
	}
	if ir.Imports[1].Path != "./helpers" || ir.Imports[1].Alias != "helpers" {
		t.Errorf("namespace import = %+v", ir.Imports[1])
	}
	if ir.Imports[2].Path != "client" || ir.Imports[2].Alias != "DefaultClient" {
		t.Errorf("default import = %+v", ir.Imports[2])
	}
	if ir.Imports[3].Path != "./types" || !ir.Imports[3].Wildcard {
		t.Errorf("export wildcard = %+v", ir.Imports[3])
	}
}

func TestTypeFacts(t *testing.T) {
	ir := parse(t, "repo.ts", fixture)

	if got := ir.Types.Vars[lang.VarKey("repo.ts:UserRepo.save", "this")]; got != "UserRepo" {
		t.Errorf("this type = %q, want UserRepo", got)
	}
	if got := ir.Types.Fields["UserRepo.db"]; got != "DB" {
		t.Errorf("field from type annotation = %q, want DB", got)
	}
	if got := ir.Types.Fields["UserRepo.cache"]; got != "Cache" {
		t.Errorf("field from new expression = %q, want Cache", got)
	}
	if got := ir.Types.Vars[lang.VarKey("repo.ts:UserRepo.save", "u")]; got != "User" {
		t.Errorf("param type = %q, want User", got)
	}
	if got := ir.Types.Vars[lang.VarKey("repo.ts:main", "repo")]; got != "UserRepo" {
		t.Errorf("var from new expression = %q, want UserRepo", got)
	}

	bases := ir.Types.Bases["UserRepo"]
	if len(bases) != 2 || bases[0] != "Base" || bases[1] != "Store" {
		t.Errorf("UserRepo bases = %+v, want [Base Store]", bases)
	}

	methods := ir.Types.Methods["UserRepo"]
	if len(methods) < 2 {
		t.Errorf("UserRepo methods = %+v", methods)
	}
}

func TestRefs(t *testing.T) {
	ir := parse(t, "repo.ts", fixture)

	var foundCall bool
	for _, r := range ir.Refs {
		if r.FromID == "repo.ts:UserRepo.save" && r.Name == "write" && r.Receiver == "this.db" {
			foundCall = true
			break
		}
	}
	if !foundCall {
		t.Errorf("did not find this.db.write ref in UserRepo.save; refs: %+v", ir.Refs)
	}
}

func TestModulePath(t *testing.T) {
	p := Plugin{}
	tests := []struct {
		root string
		file string
		want string
	}{
		{"/repo", "/repo/src/auth/service.ts", "src/auth/service"},
		{"/repo", "/repo/src/utils/index.ts", "src/utils"},
		{"/repo", "/repo/components/Button.tsx", "components/Button"},
		{"/repo", "/repo/lib/math.js", "lib/math"},
	}
	for _, tt := range tests {
		if got := p.ModulePath(tt.root, tt.file); got != tt.want {
			t.Errorf("ModulePath(%q, %q) = %q, want %q", tt.root, tt.file, got, tt.want)
		}
	}
}

func TestTSXParsing(t *testing.T) {
	tsxSrc := `import React from 'react';
export const Button = ({ label }: { label: string }) => {
  return <button onClick={() => console.log(label)}>{label}</button>;
};
`
	ir := parse(t, "Button.tsx", tsxSrc)
	if len(ir.Nodes) == 0 {
		t.Fatalf("expected nodes in tsx, got none")
	}
	if ir.Nodes[0].ID != "Button.tsx:Button" {
		t.Errorf("got node ID %q, want Button.tsx:Button", ir.Nodes[0].ID)
	}
}

func TestDeterminism(t *testing.T) {
	ir1 := parse(t, "repo.ts", fixture)
	ir2 := parse(t, "repo.ts", fixture)

	if !reflect.DeepEqual(ir1, ir2) {
		t.Errorf("two parses of the same source produced different IR")
	}
}
