package project

import "testing"

func TestMakeNames(t *testing.T) {
	tests := []struct {
		raw                                string
		label, pascal, camel, snake, kebab string
	}{
		{"orderItem", "Order item", "OrderItem", "orderItem", "order_item", "order-item"},
		{"order_item", "Order item", "OrderItem", "orderItem", "order_item", "order-item"},
		{"OrderItem", "Order item", "OrderItem", "orderItem", "order_item", "order-item"},
		{"HTTPServer", "Http server", "HTTPServer", "httpServer", "http_server", "http-server"},
		{"user_id", "User id", "UserID", "userID", "user_id", "user-id"},
		{"id", "Id", "ID", "id", "id", "id"},
		{"address2", "Address2", "Address2", "address2", "address2", "address2"},
		{"admin-htmx", "Admin htmx", "AdminHtmx", "adminHtmx", "admin_htmx", "admin-htmx"},
	}
	for _, tt := range tests {
		n, ok := makeNames(tt.raw)
		if !ok {
			t.Errorf("makeNames(%q) failed", tt.raw)
			continue
		}
		got := [...]string{n.Label, n.Pascal, n.Camel, n.Snake, n.Kebab}
		want := [...]string{tt.label, tt.pascal, tt.camel, tt.snake, tt.kebab}
		if got != want {
			t.Errorf("makeNames(%q) = %q, want %q", tt.raw, got, want)
		}
	}
}

func TestMakeNamesRejectsNonASCII(t *testing.T) {
	for _, raw := range []string{"preço", "naïveValue", "продукт"} {
		if _, ok := makeNames(raw); ok {
			t.Errorf("expected %q to be rejected", raw)
		}
	}
}

func TestPluralize(t *testing.T) {
	tests := map[string]string{
		"product": "products", "category": "categories", "day": "days",
		"box": "boxes", "address": "addresses", "branch": "branches",
		"person": "people", "child": "children", "news": "news",
		"leaf": "leaves", "hero": "heroes", "photo": "photos", "quiz": "quizzes",
	}
	for in, want := range tests {
		if got := pluralize(in); got != want {
			t.Errorf("pluralize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMakePlural(t *testing.T) {
	tests := []struct{ raw, snake, label string }{
		{"OrderItem", "order_items", "Order items"},
		{"Person", "people", "People"},
		{"HTTPServer", "http_servers", "Http servers"},
	}
	for _, tt := range tests {
		n, ok := makePlural(tt.raw)
		if !ok || n.Snake != tt.snake || n.Label != tt.label {
			t.Errorf("makePlural(%q) = %q / %q, want %q / %q", tt.raw, n.Snake, n.Label, tt.snake, tt.label)
		}
	}
}
