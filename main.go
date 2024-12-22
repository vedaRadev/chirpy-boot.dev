package main

import (
    "net/http"
    "sync/atomic"
    "fmt"
)

type ApiConfig struct { FileServerHits atomic.Int32 }

func (cfg *ApiConfig) MiddlewareMetricsInc(next http.Handler) http.Handler {
    return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
        cfg.FileServerHits.Add(1)
        next.ServeHTTP(res, req)
    })
}

func (cfg *ApiConfig) MiddlewareMetricsReset(next http.Handler) http.Handler {
    cfg.FileServerHits.Store(0)
    return next
}

func (cfg *ApiConfig) MetricsHandler(res http.ResponseWriter, req *http.Request) {
    res.WriteHeader(http.StatusOK)
    res.Header().Add("Content-Type", "text/plain; charset=utf-8")
    res.Write([]byte(fmt.Sprintf("Hits: %v", cfg.FileServerHits.Load())))
}

func (cfg *ApiConfig) ResetHandler(res http.ResponseWriter, req *http.Request) {
    cfg.FileServerHits.Store(0)
    res.WriteHeader(http.StatusOK)
    res.Write([]byte(http.StatusText(http.StatusOK)))
}

func main() {
    apiCfg := ApiConfig {}

    serveMux := http.NewServeMux()
    serveMux.Handle("/app/", apiCfg.MiddlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir("site")))))
    serveMux.HandleFunc("/healthz", func(res http.ResponseWriter, req *http.Request) {
        res.WriteHeader(http.StatusOK)
        res.Header().Add("Content-Type", "text/plain; charset=utf-8")
        res.Write([]byte("OK"))
    })
    serveMux.HandleFunc("/metrics", apiCfg.MetricsHandler)
    serveMux.HandleFunc("/reset", apiCfg.ResetHandler)

    server := http.Server { Handler: serveMux, Addr: ":8080" }
    server.ListenAndServe()
}
