package generic

import "testing"

func TestRuby(t *testing.T) {
	ir := parse(t, "lib/cart.rb", `
module Shop
  class Cart < Base
    def initialize(repo)
      @repo = repo
    end

    def total
      items.map { |i| i.price }.sum
    end

    def save
      validate(self)
      @repo.store(self)
      Audit.log("saved")
    end

    def self.build(attrs)
      new(attrs)
    end

    def _secret; end
  end
end

def test_cart_total
  Shop::Cart.build({}).total
end
`)
	assertIR(t, ir, `
node class lib/cart.rb:Shop exported
node class lib/cart.rb:Cart exported
node method lib/cart.rb:Cart.initialize exported
node method lib/cart.rb:Cart.total exported
node method lib/cart.rb:Cart.save exported
node method lib/cart.rb:Cart.build exported
node method lib/cart.rb:Cart._secret private
node function lib/cart.rb:test_cart_total exported test
ref inherits Cart -> Base
ref calls Cart.total -> <expr>.sum
ref calls Cart.total -> items.map
ref calls Cart.total -> i.price
ref calls Cart.save -> validate
ref calls Cart.save -> @repo.store
ref calls Cart.save -> Audit.log
ref calls Cart.build -> new
ref calls test_cart_total -> <expr>.total
ref calls test_cart_total -> Cart.build
`)
}

func TestPHP(t *testing.T) {
	ir := parse(t, "src/OrderService.php", `<?php
namespace Shop;

interface Auditable { public function audit(); }

class OrderService extends BaseService implements Auditable
{
    private $repo;

    public function __construct(OrderRepo $repo) { $this->repo = $repo; }

    public function place(Cart $cart)
    {
        $this->validate($cart);
        $order = new Order($cart);
        $this->repo->save($order);
        Logger::info("placed");
        return format_order($order);
    }

    private function validate(Cart $cart) {}

    public function audit() {}
}

function format_order($order) { return strval($order); }

class OrderServiceTest
{
    public function testPlace() { $s = new OrderService(null); $s->place(null); }
}
`)
	assertIR(t, ir, `
node interface src/OrderService.php:Auditable exported
node method src/OrderService.php:Auditable.audit exported
node class src/OrderService.php:OrderService exported
node method src/OrderService.php:OrderService.__construct exported
node method src/OrderService.php:OrderService.place exported
node method src/OrderService.php:OrderService.validate private
node method src/OrderService.php:OrderService.audit exported
node function src/OrderService.php:format_order exported
node class src/OrderService.php:OrderServiceTest exported
node method src/OrderService.php:OrderServiceTest.testPlace exported test
ref inherits OrderService -> BaseService
ref inherits OrderService -> Auditable
ref calls OrderService.place -> this.validate
ref calls OrderService.place -> Order
ref calls OrderService.place -> this.repo.save
ref calls OrderService.place -> Logger.info
ref calls OrderService.place -> format_order
ref calls format_order -> strval
ref calls OrderServiceTest.testPlace -> OrderService
ref calls OrderServiceTest.testPlace -> s.place
`)
}

func TestKotlin(t *testing.T) {
	ir := parse(t, "src/Cart.kt", `
package shop

interface Priced { fun price(): Long }

open class Cart(private val repo: Repo) : Base(), Priced {
    fun total(): Long {
        return items.sumOf { it.price() }
    }

    private fun save() {
        validate(this)
        repo.store(this)
        Audit.log("saved")
    }

    override fun price(): Long = total()
}

object Audit {
    fun log(msg: String) { println(msg) }
}

fun main() {
    val cart = Cart(Repo())
    cart.total()
}

class CartTest {
    @Test
    fun totals() { Cart(Repo()).total() }
}
`)
	assertIR(t, ir, `
node class src/Cart.kt:Priced exported
node method src/Cart.kt:Priced.price exported
node class src/Cart.kt:Cart exported
node method src/Cart.kt:Cart.total exported
node method src/Cart.kt:Cart.save private
node method src/Cart.kt:Cart.price exported
node class src/Cart.kt:Audit exported
node method src/Cart.kt:Audit.log exported
node function src/Cart.kt:main exported main
node class src/Cart.kt:CartTest exported
node method src/Cart.kt:CartTest.totals exported test
ref inherits Cart -> Base
ref inherits Cart -> Priced
ref calls Cart.total -> items.sumOf
ref calls Cart.total -> it.price
ref calls Cart.save -> validate
ref calls Cart.save -> repo.store
ref calls Cart.save -> Audit.log
ref calls Cart.price -> total
ref calls Audit.log -> println
ref calls main -> Cart
ref calls main -> Repo
ref calls main -> cart.total
ref calls CartTest.totals -> Cart
ref calls CartTest.totals -> Repo
ref calls CartTest.totals -> <expr>.total
`)
}
