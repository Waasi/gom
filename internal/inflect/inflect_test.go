package inflect

import "testing"

func TestCases(t *testing.T) {
	tests := []struct{ in, pascal, camel, snake, lower string }{
		{"invoice", "Invoice", "invoice", "invoice", "invoice"},
		{"invoice_item", "InvoiceItem", "invoiceItem", "invoice_item", "invoiceitem"},
		{"InvoiceItem", "InvoiceItem", "invoiceItem", "invoice_item", "invoiceitem"},
		{"company_id", "CompanyID", "companyId", "company_id", "companyid"},
		{"HTTPServerURL", "HTTPServerURL", "httpServerUrl", "http_server_url", "httpserverurl"},
		{"rate-limit", "RateLimit", "rateLimit", "rate_limit", "ratelimit"},
		{"acl", "ACL", "acl", "acl", "acl"},
		{"dueAt", "DueAt", "dueAt", "due_at", "dueat"},
		{"watcher_ids", "WatcherIDs", "watcherIds", "watcher_ids", "watcherids"},
		{"status", "Status", "status", "status", "status"},
	}
	for _, tt := range tests {
		if got := Pascal(tt.in); got != tt.pascal {
			t.Errorf("Pascal(%q) = %q, want %q", tt.in, got, tt.pascal)
		}
		if got := Camel(tt.in); got != tt.camel {
			t.Errorf("Camel(%q) = %q, want %q", tt.in, got, tt.camel)
		}
		if got := Snake(tt.in); got != tt.snake {
			t.Errorf("Snake(%q) = %q, want %q", tt.in, got, tt.snake)
		}
		if got := Lower(tt.in); got != tt.lower {
			t.Errorf("Lower(%q) = %q, want %q", tt.in, got, tt.lower)
		}
	}
}

func TestPluralSingular(t *testing.T) {
	pairs := [][2]string{
		{"contact", "contacts"}, {"company", "companies"}, {"key", "keys"},
		{"address", "addresses"}, {"box", "boxes"}, {"match", "matches"},
		{"status", "statuses"}, {"person", "people"}, {"child", "children"},
		{"invoice_item", "invoice_items"}, {"activity_record", "activity_records"},
		{"data", "data"}, {"index", "indexes"}, {"wish", "wishes"},
	}
	for _, p := range pairs {
		if got := Pluralize(p[0]); got != p[1] {
			t.Errorf("Pluralize(%q) = %q, want %q", p[0], got, p[1])
		}
		if got := Singularize(p[1]); got != p[0] {
			t.Errorf("Singularize(%q) = %q, want %q", p[1], got, p[0])
		}
	}
	// Already-plural irregulars stay put.
	if got := Pluralize("people"); got != "people" {
		t.Errorf("Pluralize(people) = %q", got)
	}
	for _, w := range []string{"person", "contact", "class", "bus"} {
		if got := Singularize(w); got != w {
			t.Errorf("Singularize(%q) = %q, want unchanged", w, got)
		}
	}
}
