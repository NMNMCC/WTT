package server

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		status := ww.Status()

		bodyR := r.Body
		var body []byte
		if bodyR != nil {
			bodyR.Read(body)
		}

		slog.Info("request", "uri", r.RequestURI, "method", r.Method, "status", status, "from", r.RemoteAddr, "body", body)
	})
}
