package generic

import "testing"

func TestCSharp(t *testing.T) {
	ir := parse(t, "src/OrderService.cs", `
using System;

namespace Shop
{
    public interface IAuditable { void Audit(); }

    public class OrderService : BaseService, IAuditable
    {
        private readonly OrderRepo _repo;

        public OrderService(OrderRepo repo) { _repo = repo; }

        public Order Place(Cart cart)
        {
            Validate(cart);
            var order = new Order(cart);
            _repo.Save(order);
            return order;
        }

        private void Validate(Cart cart)
        {
            int Count() => cart.Items.Count();
            Console.WriteLine(Count());
        }

        public void Audit() { }

        public static void Main(string[] args)
        {
            new OrderService(null).Place(null);
        }
    }

    public class OrderServiceTests
    {
        [Fact]
        public void PlacesOrder() { Helper.Setup(); }
    }
}
`)
	assertIR(t, ir, `
node interface src/OrderService.cs:IAuditable exported
node method src/OrderService.cs:IAuditable.Audit private
node class src/OrderService.cs:OrderService exported
node method src/OrderService.cs:OrderService.<init> exported
node method src/OrderService.cs:OrderService.Place exported
node method src/OrderService.cs:OrderService.Validate private
node function src/OrderService.cs:Count private
node method src/OrderService.cs:OrderService.Audit exported
node method src/OrderService.cs:OrderService.Main exported main
node class src/OrderService.cs:OrderServiceTests exported
node method src/OrderService.cs:OrderServiceTests.PlacesOrder exported test
ref inherits OrderService -> BaseService
ref inherits OrderService -> IAuditable
ref calls OrderService.Place -> Validate
ref calls OrderService.Place -> Order
ref calls OrderService.Place -> _repo.Save
ref calls Count -> cart.Items.Count
ref calls OrderService.Validate -> Console.WriteLine
ref calls OrderService.Validate -> Count
ref calls OrderService.Main -> OrderService
ref calls OrderService.Main -> <expr>.Place
ref calls OrderServiceTests.PlacesOrder -> Helper.Setup
`)
}
