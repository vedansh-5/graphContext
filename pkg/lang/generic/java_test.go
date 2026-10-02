package generic

import (
	"testing"

	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/resolver"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func TestJava(t *testing.T) {
	ir := parse(t, "src/OrderService.java", `
package shop;

import java.util.List;

public class OrderService extends BaseService implements Auditable, Comparable<OrderService> {
    private final OrderRepo repo;

    public OrderService(OrderRepo repo) {
        this.repo = repo;
        init();
    }

    public Order place(Cart cart) {
        validate(cart);
        Order order = new Order(cart);
        this.repo.save(order);
        return order;
    }

    public Order place(Cart cart, String coupon) {
        return place(cart);
    }

    private void validate(Cart cart) {
        cart.items().forEach(i -> check(i));
    }

    public static void main(String[] args) {
        new OrderService(null).place(null);
    }

    interface Auditable {
        void audit();
    }
}

class OrderServiceTest {
    @Test
    void placesOrder() {
        Helper.setup();
    }
}
`)
	assertIR(t, ir, `
node class src/OrderService.java:OrderService exported
node method src/OrderService.java:OrderService.<init> exported
node method src/OrderService.java:OrderService.place exported
node method src/OrderService.java:OrderService.place#2 exported
node method src/OrderService.java:OrderService.validate private
node method src/OrderService.java:OrderService.main exported main
node interface src/OrderService.java:Auditable private
node method src/OrderService.java:Auditable.audit private
node class src/OrderService.java:OrderServiceTest private
node method src/OrderService.java:OrderServiceTest.placesOrder private test
ref inherits OrderService -> Auditable
ref inherits OrderService -> BaseService
ref inherits OrderService -> Comparable
ref calls OrderService.<init> -> init
ref calls OrderService.place -> validate
ref calls OrderService.place -> Order
ref calls OrderService.place -> this.repo.save
ref calls OrderService.place#2 -> place
ref calls OrderService.validate -> <expr>.forEach
ref calls OrderService.validate -> check
ref calls OrderService.validate -> cart.items
ref calls OrderService.main -> <expr>.place
ref calls OrderService.main -> OrderService
ref calls OrderServiceTest.placesOrder -> Helper.setup
`)
}

// Calls resolve across Java files by name, since the engine has no imports.
func TestJavaResolvesAcrossFiles(t *testing.T) {
	svc := parse(t, "Service.java", `
class Service {
    private Repo repo;
    void run() { repo.save(); helper(); }
    void helper() {}
}
`)
	repo := parse(t, "Repo.java", `
class Repo extends Base {
    void save() {}
}
`)
	base := parse(t, "Base.java", `class Base {}`)

	res, err := resolver.Resolve("", []*lang.FileIR{svc, repo, base})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	got := map[string]store.Confidence{}
	for _, e := range res.Edges {
		got[string(e.Kind)+" "+e.SourceID+" -> "+e.TargetID] = e.Confidence
	}
	want := map[string]store.Confidence{
		"calls Service.java:Service.run -> Repo.java:Repo.save":         store.ConfNameMatch,
		"calls Service.java:Service.run -> Service.java:Service.helper": store.ConfExact,
		"inherits Repo.java:Repo -> Base.java:Base":                     store.ConfExact,
	}
	for k, conf := range want {
		if got[k] != conf {
			t.Errorf("edge %q: confidence %q, want %q\nall edges: %v", k, got[k], conf, got)
		}
	}
}
