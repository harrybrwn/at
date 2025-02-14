package xrpc

import (
	"io"
	"log/slog"
	"net/http"
)

type Pipethrough struct {
	Host   string
	Client *http.Client
	Logger *slog.Logger
}

func NewPipethrough(host string) *Pipethrough {
	return &Pipethrough{
		Host:   host,
		Client: http.DefaultClient,
		Logger: slog.Default(),
	}
}

// Hop-by-hop headers. These are removed when sent to the backend.
// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
var hopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te", // canonicalized version of "TE"
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

func cleanHeaders(header http.Header) {
	for _, h := range hopHeaders {
		header.Del(h)
	}
}

func (p *Pipethrough) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r := *req
	r.RequestURI = ""
	r.Host = p.Host
	r.URL.Host = p.Host
	cleanHeaders(r.Header)
	response, err := p.Client.Do(&r)
	if err != nil {
		p.Logger.Error("failed to pipe request", "error", err)
		w.WriteHeader(500)
		return
	}
	defer response.Body.Close()
	cleanHeaders(response.Header)
	for k, v := range response.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(response.StatusCode)
	_, err = io.Copy(w, response.Body)
	if err != nil {
		p.Logger.Error("failed to copy proxied body to response body", "error", err)
		return
	}
}
