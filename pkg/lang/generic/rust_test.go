package generic

import (
	"testing"

	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/resolver"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func TestRust(t *testing.T) {
	ir := parse(t, "src/cart.rs", `
use std::fmt::Display;

pub trait Priced: Display {
    fn price(&self) -> u64;
}

pub struct Cart {
    items: Vec<Item>,
}

struct Item;

impl Cart {
    pub fn new() -> Self {
        Cart { items: Vec::new() }
    }

    pub fn total(&self) -> u64 {
        self.items.iter().map(|i| i.price()).sum()
    }

    fn check(&self) {
        validate(self);
        helpers::audit::<Cart>(self);
    }
}

impl Priced for Cart {
    fn price(&self) -> u64 {
        self.total()
    }
}

fn validate(cart: &Cart) {}

fn main() {
    let cart = Cart::new();
    cart.check();
    println!("{}", cart.total());
}

#[cfg(test)]
mod tests {
    #[test]
    fn totals() {
        super::validate(&super::Cart::new());
    }
}
`)
	assertIR(t, ir, `
node interface src/cart.rs:Priced exported
node method src/cart.rs:Priced.price private
node class src/cart.rs:Cart exported
node class src/cart.rs:Item private
node method src/cart.rs:Cart.new exported
node method src/cart.rs:Cart.total exported
node method src/cart.rs:Cart.check private
node method src/cart.rs:Cart.price private
node function src/cart.rs:validate private
node function src/cart.rs:main private main
node function src/cart.rs:totals private test
ref inherits Priced -> Display
ref implements Cart -> Priced
ref calls Cart.new -> Vec.new
ref calls Cart.total -> <expr>.sum
ref calls Cart.total -> <expr>.map
ref calls Cart.total -> self.items.iter
ref calls Cart.total -> i.price
ref calls Cart.check -> validate
ref calls Cart.check -> helpers.audit
ref calls Cart.price -> self.total
ref calls main -> Cart.new
ref calls main -> cart.check
ref calls totals -> super.validate
ref calls totals -> Cart.new
`)
}

// "Type::method()" resolves to that type's method even when other types have
// a method of the same name, and an impl for a type from another file does not
// produce an edge with a missing source.
func TestRustStaticCallAndForeignImpl(t *testing.T) {
	a := parse(t, "a.rs", `
pub struct Cart;
impl Cart { pub fn new() -> Self { Cart } }
pub struct Order;
impl Order { pub fn new() -> Self { Order } }
`)
	b := parse(t, "b.rs", `
pub trait Priced { fn price(&self) -> u64; }
impl Priced for Cart { fn price(&self) -> u64 { 0 } }
fn build() { let c = Cart::new(); }
`)
	for _, r := range b.Refs {
		if r.Kind == store.EdgeImplements {
			t.Errorf("impl for a type declared elsewhere should not emit a ref: %+v", r)
		}
	}
	if got := b.Types.Bases["Cart"]; len(got) != 1 || got[0] != "Priced" {
		t.Errorf("Bases[Cart] = %v, want [Priced]", got)
	}

	res, err := resolver.Resolve("", []*lang.FileIR{a, b})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	var targets []string
	for _, e := range res.Edges {
		if e.SourceID == "b.rs:build" && e.Kind == store.EdgeCalls {
			targets = append(targets, e.TargetID+" "+string(e.Confidence))
		}
	}
	if len(targets) != 1 || targets[0] != "a.rs:Cart.new exact" {
		t.Errorf("Cart::new() resolved to %v, want only a.rs:Cart.new exact", targets)
	}
}
