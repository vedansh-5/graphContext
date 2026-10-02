package generic

import "testing"

func TestC(t *testing.T) {
	ir := parse(t, "src/list.c", `
#include "list.h"

struct node {
    int value;
    struct node *next;
};

static struct node *alloc_node(int value) {
    struct node *n = malloc(sizeof(struct node));
    n->value = value;
    return n;
}

void list_push(struct list *l, int value) {
    struct node *n = alloc_node(value);
    l->ops->insert(l, n);
}

int main(int argc, char **argv) {
    list_push(NULL, 1);
    return 0;
}
`)
	assertIR(t, ir, `
node class src/list.c:node exported
node function src/list.c:alloc_node private
node function src/list.c:list_push exported
node function src/list.c:main exported main
ref calls alloc_node -> malloc
ref calls list_push -> alloc_node
ref calls list_push -> l.ops.insert
ref calls main -> list_push
`)
}

func TestCpp(t *testing.T) {
	ir := parse(t, "src/cart.cpp", `
#include "cart.hpp"

namespace shop {

class Cart : public Base, private Audited {
public:
    Cart() { init(); }
    int total() const { return sum(items_); }
    void add(Item item);
private:
    std::vector<Item> items_;
};

void Cart::add(Item item) {
    items_.push_back(item);
    this->validate();
    util::log("added");
}

static int sum(const std::vector<Item>& items) { return 0; }

}  // namespace shop

int main() {
    auto *cart = new shop::Cart();
    cart->add(Item{});
    return cart->total();
}
`)
	assertIR(t, ir, `
node class src/cart.cpp:Cart exported
node method src/cart.cpp:Cart.Cart exported
node method src/cart.cpp:Cart.total exported
node method src/cart.cpp:Cart.add exported
node function src/cart.cpp:sum private
node function src/cart.cpp:main exported main
ref inherits Cart -> Base
ref inherits Cart -> Audited
ref calls Cart.Cart -> init
ref calls Cart.total -> sum
ref calls Cart.add -> items_.push_back
ref calls Cart.add -> this.validate
ref calls Cart.add -> util.log
ref calls main -> Cart
ref calls main -> cart.add
ref calls main -> cart.total
`)
}
