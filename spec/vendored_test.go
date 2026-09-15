package spec

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// Both vendored documents are fetched from whichever host the
// nightly job asks and then stripped of what that host put in
// them: the specification loses the tenant's title, contact,
// servers and token URL, and every tool description loses the
// trailing link into that host's guide. The stripping happens in
// the workflow, so nothing but these tests stands between a
// broken pattern and a hostname committed into spec/.

// neutralHosts are the hosts a vendored document may name: the
// vendor's own site, the placeholder the prose uses, the example
// domains reserved for documentation, and the loopback addresses
// of the redirect URI rules. A tenant host is none of them.
func neutralHost(host string) bool {
	switch host {
	case "transpareo.com", "<host>", "localhost", "127.0.0.1",
		"example.com", "assistant.example":
		return true
	}
	return strings.HasSuffix(host, ".example.com") ||
		strings.HasSuffix(host, ".example")
}

// The angle brackets stay in, so the documented https://<host>/api
// form yields the placeholder rather than a truncation of it.
var urlPattern = regexp.MustCompile("https?://[^\\s\"'`\\\\)\\]},]+")

// hostsIn lists the host of every absolute URL in the document,
// with the port taken off.
func hostsIn(document []byte) []string {
	var hosts []string
	for _, match := range urlPattern.FindAllString(string(document), -1) {
		rest := match[strings.Index(match, "://")+3:]
		if i := strings.IndexAny(rest, "/?#"); i >= 0 {
			rest = rest[:i]
		}
		rest = strings.TrimRight(rest, ".")
		if i := strings.LastIndex(rest, ":"); i >= 0 {
			rest = rest[:i]
		}
		if rest != "" {
			hosts = append(hosts, rest)
		}
	}
	return hosts
}

func TestVendoredDocumentsNameNoRealHost(t *testing.T) {
	for _, document := range []struct {
		name string
		body []byte
	}{{"openapi.json", JSON}, {"mcp-tools.json", CatalogueJSON}} {
		seen := map[string]bool{}
		for _, host := range hostsIn(document.body) {
			if neutralHost(host) || seen[host] {
				continue
			}
			seen[host] = true
			t.Errorf("spec/%s names the host %q; the vendored document "+
				"must carry no host the fetch happened to reach",
				document.name, host)
		}
	}
}

// The sweep is only worth having if it would catch the leak it is
// there for, so it is run against the shape a broken strip leaves
// behind.
func TestARealHostIsNotNeutral(t *testing.T) {
	leak := []byte(`{"description":"Void a DPP\nSee ` +
		`https://apicheck.transpareo.dev/apidocs/guide#dpps"}`)
	hosts := hostsIn(leak)
	if len(hosts) != 1 || hosts[0] != "apicheck.transpareo.dev" {
		t.Fatalf("hostsIn = %v, want the one host of the link", hosts)
	}
	if neutralHost(hosts[0]) {
		t.Errorf("%q counts as neutral, so a leaked host would pass",
			hosts[0])
	}
	for _, host := range []string{"transpareo.com", "<host>", "example.com",
		"cdn.example.com", "assistant.example", "localhost", "127.0.0.1"} {
		if !neutralHost(host) {
			t.Errorf("%q counts as a real host, so the documents cannot "+
				"use it as a placeholder", host)
		}
	}
}

// The strip takes away the whole "See <url>" line. A pattern that
// stops matching leaves the link in place, and the link names the
// host that answered.
func TestVendoredCatalogueKeepsNoGuideLinks(t *testing.T) {
	if strings.Contains(string(CatalogueJSON), "apidocs/guide") {
		t.Error("spec/mcp-tools.json still links into a host's guide; the " +
			"nightly job's strip has stopped matching")
	}
}

func TestVendoredSpecificationIsNeutralised(t *testing.T) {
	var doc struct {
		Info struct {
			Title   string `json:"title"`
			Contact struct {
				Name string `json:"name"`
				URL  string `json:"url"`
			} `json:"contact"`
		} `json:"info"`
		Servers []struct {
			URL string `json:"url"`
		} `json:"servers"`
		Components struct {
			SecuritySchemes struct {
				OAuth2 struct {
					Flows struct {
						ClientCredentials struct {
							TokenURL string `json:"tokenUrl"`
						} `json:"clientCredentials"`
					} `json:"flows"`
				} `json:"oauth2"`
			} `json:"securitySchemes"`
		} `json:"components"`
	}
	if err := json.Unmarshal(JSON, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, field := range []struct{ name, got, want string }{
		{"info.title", doc.Info.Title, "Transpareo API"},
		{"info.contact.name", doc.Info.Contact.Name, "Transpareo Support"},
		{"info.contact.url", doc.Info.Contact.URL, "https://transpareo.com"},
		{"the token URL",
			doc.Components.SecuritySchemes.OAuth2.Flows.ClientCredentials.
				TokenURL, "/api/oauth/token"},
	} {
		if field.got != field.want {
			t.Errorf("%s is %q, want the neutral %q", field.name, field.got,
				field.want)
		}
	}
	if len(doc.Servers) != 1 || doc.Servers[0].URL != "/api" {
		t.Errorf("servers is %+v, want the single relative /api", doc.Servers)
	}
}
