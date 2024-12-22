package main

import (
    "net/http"
)

func main() {
    serveMux := http.NewServeMux()
    serveMux.Handle("/app/", http.StripPrefix("/app", http.FileServer(http.Dir("site"))))
    serveMux.HandleFunc("/healthz", func(res http.ResponseWriter, req *http.Request) {
        res.WriteHeader(200)
        res.Header().Add("Content-Type", "text/plain; charset=utf-8")
        res.Write([]byte("OK"))
    })

    server := http.Server { Handler: serveMux, Addr: ":8080" }
    server.ListenAndServe()
}
