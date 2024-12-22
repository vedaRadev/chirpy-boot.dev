package main

import (
    "net/http"
    "sync/atomic"
    "encoding/json"
    "fmt"
    "strings"
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
    res.Header().Add("Content-Type", "text/html; charset=utf-8")
    html := fmt.Sprintf(
        `
        <html>
            <body>
                <h1>Welcome, Chirpy Admin</h1>
                <p>Chirpy has been visited %d times!</p>
            </body>
        </html>
        `,
        cfg.FileServerHits.Load(),
    )
    res.Write([]byte(html))
}

func (cfg *ApiConfig) ResetHandler(res http.ResponseWriter, req *http.Request) {
    cfg.FileServerHits.Store(0)
    res.WriteHeader(http.StatusOK)
    res.Write([]byte(http.StatusText(http.StatusOK)))
}

func main() {
    apiCfg := ApiConfig {}

    serveMux := http.NewServeMux()

    //============================== APP ==============================
    serveMux.Handle("/app/", apiCfg.MiddlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir("site")))))

    //============================== API ==============================
    serveMux.HandleFunc("GET /api/healthz", func(res http.ResponseWriter, req *http.Request) {
        res.WriteHeader(http.StatusOK)
        res.Header().Add("Content-Type", "text/plain; charset=utf-8")
        res.Write([]byte("OK"))
    })
    serveMux.HandleFunc("POST /api/validate_chirp", func (res http.ResponseWriter, req *http.Request) {
        type RequestParameters struct { Body string `json:"body"` }
        const MAX_CHIRP_LEN int = 140

        res.Header().Set("Content-Type", "application/json")

        var reqParams RequestParameters
        if err := json.NewDecoder(req.Body).Decode(&reqParams); err != nil {
            res.WriteHeader(http.StatusInternalServerError)
            res.Write([]byte(`{"error":"Failed to decode request json body"}`))
            return
        }

        if len(reqParams.Body) > MAX_CHIRP_LEN {
            res.WriteHeader(http.StatusBadRequest)
            res.Write([]byte(`{"error":"Chirp is too long"}`))
            return
        }

        // Filter profanity
        words := strings.Split(reqParams.Body, " ")
        for i := range words {
            lower := strings.ToLower(words[i])
            if lower == "kerfuffle" || lower == "sharbert" || lower == "fornax" {
                words[i] = "****"
            }
        }
        cleaned := strings.Join(words, " ")

        res.WriteHeader(http.StatusOK)
        res.Write([]byte(fmt.Sprintf(`{"cleaned_body":"%s"}`, cleaned)))
    })

    //============================== ADMIN ==============================
    serveMux.HandleFunc("GET /admin/metrics", apiCfg.MetricsHandler)
    serveMux.HandleFunc("POST /admin/reset", apiCfg.ResetHandler)

    server := http.Server { Handler: serveMux, Addr: ":8080" }
    server.ListenAndServe()
}
