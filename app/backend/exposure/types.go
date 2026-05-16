package exposure

// Endpoint is a browser-openable cluster exposure (NodePort or Ingress).
type Endpoint struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Label     string `json:"label"`
	URL       string `json:"url"`
	Port      int    `json:"port,omitempty"`
}

// Snapshot is the API/SSE payload.
type Snapshot struct {
	Endpoints []Endpoint `json:"endpoints"`
}
