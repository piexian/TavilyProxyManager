package services

import (
	"net/http"
	"testing"
)

func TestCopyHeadersDropsHopByHop(t *testing.T) {
	src := http.Header{}
	src.Set("Connection", "upgrade, X-Conn-Scoped")
	src.Set("Upgrade", "websocket")
	src.Set("Keep-Alive", "timeout=5")
	src.Set("Proxy-Connection", "keep-alive")
	src.Set("Te", "trailers")
	src.Set("Transfer-Encoding", "chunked")
	src.Set("X-Conn-Scoped", "drop-me")
	src.Set("Host", "example.com")
	src.Set("Content-Length", "10")
	src.Set("Content-Type", "application/json")
	src.Set("Cf-Connecting-Ip", "203.0.113.1")
	src.Add("Accept", "a")
	src.Add("Accept", "b")

	dst := http.Header{}
	copyHeaders(dst, src)

	for _, k := range []string{"Connection", "Upgrade", "Keep-Alive", "Proxy-Connection", "Te",
		"Transfer-Encoding", "X-Conn-Scoped", "Host", "Content-Length"} {
		if _, ok := dst[http.CanonicalHeaderKey(k)]; ok {
			t.Errorf("header %s must not be forwarded", k)
		}
	}
	if got := dst.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := dst.Get("Cf-Connecting-Ip"); got != "203.0.113.1" {
		t.Errorf("Cf-Connecting-Ip = %q", got)
	}
	if got := dst.Values("Accept"); len(got) != 2 {
		t.Errorf("Accept values = %v", got)
	}
}
