package domainfront

import "net/http"

type privateResponse struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *privateResponse) preventCaching() {
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	w.Header().Set("CDN-Cache-Control", "no-store")
	w.Header().Set("Cloudflare-CDN-Cache-Control", "no-store")
}

func (w *privateResponse) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.preventCaching()
	if status >= 200 || status == http.StatusSwitchingProtocols {
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *privateResponse) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.preventCaching()
		w.wroteHeader = true
	}
	// Let the underlying writer perform its implicit 200 and content sniffing.
	return w.ResponseWriter.Write(p)
}
func (w *privateResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func localHandler(handler http.Handler, host string, subscription bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		cloned := req.Clone(req.Context())
		if host != "" {
			cloned.Host = host
		}
		if subscription {
			private := &privateResponse{ResponseWriter: w}
			private.preventCaching()
			handler.ServeHTTP(private, cloned)
			if !private.wroteHeader {
				private.preventCaching()
			}
		} else {
			handler.ServeHTTP(w, cloned)
		}
	})
}
