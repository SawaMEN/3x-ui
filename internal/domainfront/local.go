package domainfront

import "net/http"

type privateResponse struct{ http.ResponseWriter }

func (w privateResponse) WriteHeader(status int) {
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	w.Header().Set("CDN-Cache-Control", "no-store")
	w.Header().Set("Cloudflare-CDN-Cache-Control", "no-store")
	w.ResponseWriter.WriteHeader(status)
}

func (w privateResponse) Write(p []byte) (int, error) {
	if w.Header().Get("Cache-Control") != "private, no-store, max-age=0" {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}
func (w privateResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func localHandler(handler http.Handler, host string, subscription bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		cloned := req.Clone(req.Context())
		if host != "" {
			cloned.Host = host
		}
		if subscription {
			handler.ServeHTTP(privateResponse{w}, cloned)
		} else {
			handler.ServeHTTP(w, cloned)
		}
	})
}
