package project

import "testing"

func TestMoney(t *testing.T) {
	src := `package catalog

app shop {
  module "example.com/shop"
  currency BRL
}

entity Product {
  price money required min(0) max(99999.99)
  fee   money default(1.5)
}
`
	p, diags := Load(Source{Name: "catalog.esq", Src: []byte(src)})
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	if p.App.Currency != "BRL" {
		t.Errorf("currency %q, want BRL", p.App.Currency)
	}
	e := p.Entities[0]
	price, fee := e.Field("price"), e.Field("fee")
	if price.Type != TypeMoney || !price.Required || price.Min == nil || price.Min.Value != 0 || price.Max == nil || price.Max.Text != "99999.99" {
		t.Errorf("price: %+v", price)
	}
	if fee.Type != TypeMoney || fee.Default == nil || fee.Default.Text != "1.5" {
		t.Errorf("fee: %+v", fee)
	}
}
