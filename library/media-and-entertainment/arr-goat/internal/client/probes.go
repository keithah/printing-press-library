package client

// probe describes the health path for REST-style services. Transmission and
// other RPC services are handled specially in client.go.
type probe struct {
	path string
}

var probes = map[string]probe{
	"sonarr":   {path: "/api/v3/health"},
	"radarr":   {path: "/api/v3/health"},
	"prowlarr": {path: "/api/v1/health"},
	"bazarr":   {path: "/api/system/status"},
	"sabnzbd":  {path: "/api?mode=queue&output=json&apikey={key}"},
}
