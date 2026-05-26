package exposure

import (
	"encoding/json"
	"testing"
)

func TestBrowserIngressPath(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "/"},
		{"/", "/"},
		{"/ctf", "/ctf/"},
		{"/ctf/", "/ctf/"},
		{"/ctf(/|$)(.*)", "/ctf/"},
		{"/api(/|$)(.*)", "/api/"},
		{"/something(/|$)(.*)", "/something/"},
	}
	for _, tc := range tests {
		got := browserIngressPath(tc.in)
		if got != tc.want {
			t.Errorf("browserIngressPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIngressLabel(t *testing.T) {
	tests := []struct {
		host, visitPath, want string
	}{
		{"localhost", "/", "localhost"},
		{"localhost", "/ctf/", "localhost/ctf"},
		{"example.com", "/api/", "example.com/api"},
	}
	for _, tc := range tests {
		got := ingressLabel(tc.host, tc.visitPath)
		if got != tc.want {
			t.Errorf("ingressLabel(%q, %q) = %q, want %q", tc.host, tc.visitPath, got, tc.want)
		}
	}
}

func TestBuildEndpointsIngressRegexPath(t *testing.T) {
	ing := `{
		"metadata": {"name": "simple-ctf-ingress", "namespace": "deployment-basics"},
		"spec": {
			"rules": [{
				"host": "localhost",
				"http": {
					"paths": [{
						"path": "/ctf(/|$)(.*)"
					}]
				}
			}]
		}
	}`
	ingresses := map[string]json.RawMessage{
		"ing/deployment-basics/simple-ctf-ingress": []byte(ing),
	}
	eps := buildEndpoints(nil, ingresses)
	if len(eps) != 1 {
		t.Fatalf("len(endpoints) = %d, want 1", len(eps))
	}
	ep := eps[0]
	if ep.Label != "localhost/ctf" {
		t.Errorf("Label = %q, want localhost/ctf", ep.Label)
	}
	if ep.URL != "http://localhost/ctf/" {
		t.Errorf("URL = %q, want http://localhost/ctf/", ep.URL)
	}
}
