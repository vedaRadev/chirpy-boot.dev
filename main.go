// TODO's
//
// Helper function for sending simple json error responses?
// I don't really see much benefit to this yet since it really doesn't cut down on code length or
// make anything more concise. It _maybe_ could save me from typos when creating the json body but
// it's literally one field and I just don't see it being an issue.

package main

import _ "github.com/lib/pq"
import (
    "net/http"
    "sync/atomic"
    "encoding/json"
    "fmt"
    "strings"
    "os"
    "database/sql"

    "github.com/joho/godotenv"

    "github.com/vedaRadev/chirpy-boot.dev/internal/database"
)

type ApiConfig struct  {
    FileServerHits atomic.Int32
    Platform string
    Db *database.Queries
}

func (cfg *ApiConfig) MiddlewareMetricsInc(next http.Handler) http.Handler {
    return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
        cfg.FileServerHits.Add(1)
        next.ServeHTTP(res, req)
    })
}

func (cfg *ApiConfig) HandleMetrics(res http.ResponseWriter, req *http.Request) {
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

func (cfg *ApiConfig) HandleReset(res http.ResponseWriter, req *http.Request) {
    if cfg.Platform != "dev" {
        res.WriteHeader(http.StatusForbidden)
        res.Write([]byte(http.StatusText(http.StatusForbidden)))
        return
    }

    // TODO should we bail entirely or continue on and reset everything we can?
    if _, err := cfg.Db.Reset(req.Context()); err != nil {
        res.WriteHeader(http.StatusInternalServerError)
        msg := fmt.Sprintf("Failed to reset database: %v", err.Error())
        fmt.Println(msg)
        res.Write([]byte(msg))
        return
    }

    cfg.FileServerHits.Store(0)
    res.WriteHeader(http.StatusOK)
    res.Write([]byte(http.StatusText(http.StatusOK)))
}

func (cfg *ApiConfig) HandleCreateUser(res http.ResponseWriter, req *http.Request) {
    type RequestParameters struct { Email string `json:"email"` }

    res.Header().Set("Content-Type", "application/json")

    var reqParams RequestParameters
    if err := json.NewDecoder(req.Body).Decode(&reqParams); err != nil {
        res.WriteHeader(http.StatusInternalServerError)
        res.Write([]byte(`{"error":"Failed to decode request json body"}`))
    }

    user, err := cfg.Db.CreateUser(req.Context(), reqParams.Email)
    if err != nil {
        res.WriteHeader(http.StatusInternalServerError)
        res.Write([]byte(`{"error":"Failed to create user"}`))
        // TODO better logging
        fmt.Printf("Failed to create user %v: %v\n", reqParams.Email, err.Error())
        return
    }

    res.WriteHeader(http.StatusCreated)
    // FIXME assuming that the json marshalling will never fail
    resBody, _ := json.Marshal(user)
    res.Write(resBody)
}

func HandleValidateChirp(res http.ResponseWriter, req *http.Request) {
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
}

func main() {
    godotenv.Load()
    dbUrl := os.Getenv("DB_URL")
    db, err := sql.Open("postgres", dbUrl)
    if err != nil {
        fmt.Println("Failed to connect to chirpy db")
        os.Exit(1)
    }
    database.New(db)
    fmt.Println("Connected to the chirpy db")
    dbQueries := database.New(db)

    serveMux := http.NewServeMux()
    platform := os.Getenv("PLATFORM")
    if platform == "" {
        fmt.Println("platform must be set")
        os.Exit(1)
    }
    apiCfg := ApiConfig { Platform: platform, Db: dbQueries }

    //============================== APP ==============================
    serveMux.Handle("/app/", apiCfg.MiddlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir("site")))))
    //============================== API ==============================
    serveMux.HandleFunc("GET /api/healthz", func(res http.ResponseWriter, req *http.Request) {
        res.WriteHeader(http.StatusOK)
        res.Header().Add("Content-Type", "text/plain; charset=utf-8")
        res.Write([]byte("OK"))
    })
    serveMux.HandleFunc("POST /api/validate_chirp", HandleValidateChirp)
    serveMux.HandleFunc("POST /api/users", apiCfg.HandleCreateUser)
    //============================== ADMIN ==============================
    serveMux.HandleFunc("GET /admin/metrics", apiCfg.HandleMetrics)
    serveMux.HandleFunc("POST /admin/reset", apiCfg.HandleReset)

    server := http.Server { Handler: serveMux, Addr: ":8080" }
    server.ListenAndServe()
}
