package exposure

import (
	"net/url"
	"os"
	"strconv"
	"strings"
)

// publicBase holds scheme and host for NodePort URLs (from K3SLAB_PUBLIC_ORIGIN).
type publicBase struct {
	Scheme string
	Host   string
}

func loadPublicBase() publicBase {
	raw := strings.TrimSpace(os.Getenv("K3SLAB_PUBLIC_ORIGIN"))
	if raw == "" {
		return publicBase{Scheme: "http", Host: "localhost"}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return publicBase{Scheme: "http", Host: "localhost"}
	}
	scheme := u.Scheme
	if scheme != "http" && scheme != "https" {
		scheme = "http"
	}
	host := u.Hostname()
	if host == "" {
		host = "localhost"
	}
	return publicBase{Scheme: scheme, Host: host}
}

func ingressHTTPPort() int {
	return envPort("K3SLAB_INGRESS_HTTP_PORT", 80)
}

func ingressHTTPSPort() int {
	return envPort("K3SLAB_INGRESS_HTTPS_PORT", 443)
}

func envPort(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 || n > 65535 {
		return def
	}
	return n
}

func formatURL(scheme, host string, port int, path string) string {
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	var authority string
	if (scheme == "http" && port == 80) || (scheme == "https" && port == 443) {
		authority = host
	} else {
		authority = host + ":" + strconv.Itoa(port)
	}
	return scheme + "://" + authority + path
}
