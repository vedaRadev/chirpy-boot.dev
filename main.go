package main

import (
    "net/http"
)

func main() {
    serveMux := http.NewServeMux()
    serveMux.Handle("/", http.FileServer(http.Dir("res")))

    server := http.Server { Handler: serveMux, Addr: ":8080" }

    server.ListenAndServe()
}
