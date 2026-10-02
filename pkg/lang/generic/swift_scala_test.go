package generic

import "testing"

func TestSwift(t *testing.T) {
	ir := parse(t, "Sources/Cart.swift", `
import Foundation

protocol Priced {
    func price() -> Int
}

class Cart: Base, Priced {
    private let repo: Repo

    init(repo: Repo) {
        self.repo = repo
        setup()
    }

    func total() -> Int {
        return items.map { $0.price() }.reduce(0, +)
    }

    private func save() {
        validate(self)
        repo.store(self)
        Audit.log("saved")
    }

    func price() -> Int { return total() }
}

extension Cart {
    func isEmpty() -> Bool { return total() == 0 }
}

struct Item { }

class CartTests: XCTestCase {
    func testTotal() {
        let cart = Cart(repo: Repo())
        XCTAssertEqual(cart.total(), 0)
    }
}
`)
	assertIR(t, ir, `
node interface Sources/Cart.swift:Priced exported
node method Sources/Cart.swift:Priced.price exported
node class Sources/Cart.swift:Cart exported
node method Sources/Cart.swift:Cart.<init> exported
node method Sources/Cart.swift:Cart.total exported
node method Sources/Cart.swift:Cart.save private
node method Sources/Cart.swift:Cart.price exported
node method Sources/Cart.swift:Cart.isEmpty exported
node class Sources/Cart.swift:Item exported
node class Sources/Cart.swift:CartTests exported
node method Sources/Cart.swift:CartTests.testTotal exported test
ref inherits Cart -> Base
ref inherits Cart -> Priced
ref inherits CartTests -> XCTestCase
ref calls Cart.<init> -> setup
ref calls Cart.total -> <expr>.reduce
ref calls Cart.total -> items.map
ref calls Cart.total -> $0.price
ref calls Cart.save -> validate
ref calls Cart.save -> repo.store
ref calls Cart.save -> Audit.log
ref calls Cart.price -> total
ref calls Cart.isEmpty -> total
ref calls CartTests.testTotal -> Cart
ref calls CartTests.testTotal -> Repo
ref calls CartTests.testTotal -> XCTAssertEqual
ref calls CartTests.testTotal -> cart.total
`)
}

func TestScala(t *testing.T) {
	ir := parse(t, "src/Cart.scala", `
package shop

trait Priced {
  def price(): Long
}

class Cart(repo: Repo) extends Base with Priced {
  def total(): Long = items.map(_.price()).sum

  private def save(): Unit = {
    validate(this)
    repo.store(this)
    Audit.log("saved")
  }

  def price(): Long = total()
}

object Audit {
  def log(msg: String): Unit = println(msg)
}

object Main {
  def main(args: Array[String]): Unit = {
    val cart = new Cart(new Repo())
    cart.total()
  }
}
`)
	assertIR(t, ir, `
node interface src/Cart.scala:Priced exported
node method src/Cart.scala:Priced.price exported
node class src/Cart.scala:Cart exported
node method src/Cart.scala:Cart.total exported
node method src/Cart.scala:Cart.save private
node method src/Cart.scala:Cart.price exported
node class src/Cart.scala:Audit exported
node method src/Cart.scala:Audit.log exported
node class src/Cart.scala:Main exported
node method src/Cart.scala:Main.main exported main
ref inherits Cart -> Base
ref inherits Cart -> Priced
ref calls Cart.total -> items.map
ref calls Cart.total -> _.price
ref calls Cart.save -> validate
ref calls Cart.save -> repo.store
ref calls Cart.save -> Audit.log
ref calls Cart.price -> total
ref calls Audit.log -> println
ref calls Main.main -> Cart
ref calls Main.main -> Repo
ref calls Main.main -> cart.total
`)
}
