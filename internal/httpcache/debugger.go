package httpcache

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

type Debugger struct {
	RoundTripper http.RoundTripper
	Logger       *slog.Logger
}

func (d *Debugger) RoundTrip(r *http.Request) (*http.Response, error) {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	d.Logger.Debug("http request",
		"method", r.Method,
		"url", r.URL.String(),
		headerGroup(r.Header))
	res, err := d.RoundTripper.RoundTrip(r)
	if err != nil {
		return res, err
	}
	var buf bytes.Buffer
	_, err = io.Copy(&buf, res.Body)
	if err != nil {
		return res, err
	}
	body := buf.String()
	res.Body = io.NopCloser(&buf)
	d.Logger.Debug("http response",
		"status", res.Status,
		"url", res.Request.URL.String(),
		"len", res.ContentLength,
		"body", body,
		headerGroup(res.Header))
	return res, nil
}

func headerGroup(header http.Header) slog.Attr {
	args := make([]any, 0, len(header))
	for k, v := range header {
		if strings.ToLower(k) == "authorization" {
			continue
		}
		args = append(args, slog.String(k, strings.Join(v, ",")))
	}
	return slog.Group("headers", args...)
}
