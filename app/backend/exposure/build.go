package exposure

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

type meta struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type serviceObject struct {
	Metadata meta `json:"metadata"`
	Spec     struct {
		Type  string `json:"type"`
		Ports []struct {
			NodePort int `json:"nodePort"`
		} `json:"ports"`
	} `json:"spec"`
}

type ingressObject struct {
	Metadata meta `json:"metadata"`
	Spec     struct {
		TLS []struct {
			Hosts []string `json:"hosts"`
		} `json:"tls"`
		Rules []struct {
			Host string `json:"host"`
			HTTP *struct {
				Paths []struct {
					Path string `json:"path"`
				} `json:"paths"`
			} `json:"http"`
		} `json:"rules"`
	} `json:"spec"`
}

func buildEndpoints(services map[string]json.RawMessage, ingresses map[string]json.RawMessage) []Endpoint {
	base := loadPublicBase()
	var out []Endpoint

	for _, raw := range services {
		var svc serviceObject
		if json.Unmarshal(raw, &svc) != nil {
			continue
		}
		if svc.Spec.Type != "NodePort" {
			continue
		}
		ns, name := svc.Metadata.Namespace, svc.Metadata.Name
		for _, p := range svc.Spec.Ports {
			if p.NodePort <= 0 {
				continue
			}
			id := "nodeport/" + ns + "/" + name + "/" + strconv.Itoa(p.NodePort)
			out = append(out, Endpoint{
				ID:        id,
				Kind:      "nodeport",
				Namespace: ns,
				Name:      name,
				Label:     name + ":" + strconv.Itoa(p.NodePort),
				URL:       formatURL(base.Scheme, base.Host, p.NodePort, "/"),
				Port:      p.NodePort,
			})
		}
	}

	httpPort := ingressHTTPPort()
	httpsPort := ingressHTTPSPort()
	for _, raw := range ingresses {
		var ing ingressObject
		if json.Unmarshal(raw, &ing) != nil {
			continue
		}
		ns, name := ing.Metadata.Namespace, ing.Metadata.Name
		for _, rule := range ing.Spec.Rules {
			host := strings.TrimSpace(rule.Host)
			if host == "" {
				continue
			}
			paths := []string{"/"}
			if rule.HTTP != nil && len(rule.HTTP.Paths) > 0 {
				paths = nil
				for _, p := range rule.HTTP.Paths {
					pp := p.Path
					if pp == "" {
						pp = "/"
					}
					paths = append(paths, pp)
				}
			}
			tls := hostHasTLS(host, ing.Spec.TLS)
			for _, path := range paths {
				httpID := ingressID(ns, name, host, path, "http")
				label := host
				if path != "/" {
					label = host + path
				}
				out = append(out, Endpoint{
					ID:        httpID,
					Kind:      "ingress",
					Namespace: ns,
					Name:      name,
					Label:     label,
					URL:       formatURL("http", host, httpPort, path),
					Port:      httpPort,
				})
				if tls {
					out = append(out, Endpoint{
						ID:        ingressID(ns, name, host, path, "https"),
						Kind:      "ingress",
						Namespace: ns,
						Name:      name,
						Label:     label + " (https)",
						URL:       formatURL("https", host, httpsPort, path),
						Port:      httpsPort,
					})
				}
			}
		}
	}

	if out == nil {
		out = []Endpoint{}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Port != b.Port {
			return a.Port < b.Port
		}
		return a.URL < b.URL
	})
	return out
}

func ingressID(ns, name, host, path, scheme string) string {
	p := strings.TrimPrefix(path, "/")
	return "ingress/" + ns + "/" + name + "/" + scheme + "/" + host + "/" + p
}

func hostHasTLS(host string, tlsBlocks []struct {
	Hosts []string `json:"hosts"`
}) bool {
	for _, t := range tlsBlocks {
		for _, h := range t.Hosts {
			h = strings.TrimSpace(h)
			if h == "" {
				continue
			}
			if h == host {
				return true
			}
			if strings.HasPrefix(h, "*.") {
				suffix := h[1:]
				if strings.HasSuffix(host, suffix) && host != suffix {
					return true
				}
			}
		}
	}
	return false
}
