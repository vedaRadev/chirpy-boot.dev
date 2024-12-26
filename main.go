package main

import _ "github.com/lib/pq"
import (
    "net/http"
    "sync/atomic"
    "encoding/json"
    "fmt"
    "os"
    "database/sql"
    "errors"

    "github.com/joho/godotenv"
    "github.com/google/uuid"

    "github.com/vedaRadev/chirpy-boot.dev/internal/database"
    "github.com/vedaRadev/chirpy-boot.dev/internal/auth"
)

func SendJsonErrorResponse(res http.ResponseWriter, code int, message string) {
    if message == "" { message = http.StatusText(code) }
    res.Header().Set("Content-Type", "application/json")
    res.WriteHeader(code)
    res.Write([]byte(fmt.Sprintf(`{"error":"%s"}`, message)))
}

// Attempt to send a json response, send an error if something goes wrong when marshalling data
func SendJsonResponse(res http.ResponseWriter, code int, data any) {
    resBody, err := json.Marshal(data)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusInternalServerError, "failed to marshal response data")
        return
    }

    res.Header().Set("Content-Type", "application/json")
    res.WriteHeader(code)
    res.Write(resBody)
}

func GetAuthenticatedUserId(header http.Header, secret string) (uuid.UUID, error, int) {
    accessToken, err := auth.GetBearerToken(header)
    if err != nil {
        return uuid.UUID {}, err, http.StatusUnauthorized
    }
    userId, err := auth.ValidateJWT(accessToken, secret)
    if err != nil {
        return uuid.UUID {}, errors.New("failed to validate access token"), http.StatusUnauthorized
    }

    return userId, nil, 0
}

func DecodeRequestBodyParameters[T any](reqParams *T, res http.ResponseWriter, req *http.Request) (error, int) {
    if err := json.NewDecoder(req.Body).Decode(reqParams); err != nil {
        return errors.New("failed to decode request body"), http.StatusInternalServerError
    }

    return nil, 0
}

type ApiConfig struct  {
    FileServerHits atomic.Int32
    Platform string
    Secret string
    PolkaKey string
    Db *database.Queries
}

func (cfg *ApiConfig) MiddlewareMetricsInc(next http.Handler) http.Handler {
    return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
        cfg.FileServerHits.Add(1)
        next.ServeHTTP(res, req)
    })
}

func (cfg *ApiConfig) HandleMetrics(res http.ResponseWriter, req *http.Request) {
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
    res.Header().Add("Content-Type", "text/html; charset=utf-8")
    res.WriteHeader(http.StatusOK)
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

func (cfg *ApiConfig) HandlePolkaEvent(res http.ResponseWriter, req *http.Request) {
    apiKey, err := auth.GetApiKey(req.Header)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusUnauthorized, err.Error())
        return
    }
    if apiKey != cfg.PolkaKey {
        SendJsonErrorResponse(res, http.StatusUnauthorized, "incorrect api key")
        return
    }

    type RequestParameters struct {
        Event string `json:"event"`
        // TODO does data need to be an interface{} here and more finely decoded based on event type
        // later?
        Data struct {
            UserID uuid.UUID `json:"user_id"`
        } `json:"data"`
    }
    var reqParams RequestParameters
    if err, errCode := DecodeRequestBodyParameters(&reqParams, res, req); err != nil {
        SendJsonErrorResponse(res, errCode, err.Error())
        return
    }

    if reqParams.Event == "user.upgraded" {
        _, err := cfg.Db.UpgradeUserToChirpyRed(req.Context(), reqParams.Data.UserID)
        if err != nil {
            SendJsonErrorResponse(res, http.StatusNotFound, "user not found")
            return
        }
    }

    res.WriteHeader(http.StatusNoContent)
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
    platform := os.Getenv("PLATFORM")
    if platform == "" {
        fmt.Println("platform must be set")
        os.Exit(1)
    }
    secret := os.Getenv("SECRET")
    if secret == "" {
        fmt.Println("secret must be set")
        os.Exit(1)
    }
    polkaKey := os.Getenv("POLKA_KEY")
    if secret == "" {
        fmt.Println("polka key must be set")
        os.Exit(1)
    }
    apiCfg := ApiConfig {
        Platform: platform,
        PolkaKey: polkaKey,
        Secret: secret,
        Db: dbQueries,
    }

    serveMux := http.NewServeMux()
    //============================== APP ==============================
    serveMux.Handle("/app/", apiCfg.MiddlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir("site")))))
    //============================== API ==============================
    // Health
    serveMux.HandleFunc("GET /api/healthz", func(res http.ResponseWriter, req *http.Request) {
        res.WriteHeader(http.StatusOK)
        res.Header().Add("Content-Type", "text/plain; charset=utf-8")
        res.Write([]byte("OK"))
    })
    // Chirps (handlers_chirps.go)
    serveMux.HandleFunc("GET /api/chirps", apiCfg.HandleGetChirps)
    serveMux.HandleFunc("GET /api/chirps/{id}", apiCfg.HandleGetChirp)
    serveMux.HandleFunc("POST /api/chirps", apiCfg.HandleCreateChirp)
    serveMux.HandleFunc("DELETE /api/chirps/{id}", apiCfg.HandleDeleteChirp)
    // Users (handlers_users.go)
    serveMux.HandleFunc("POST /api/users", apiCfg.HandleCreateUser)
    serveMux.HandleFunc("PUT /api/users", apiCfg.HandleUpdateUser)
    // Auth (handlers_users.go)
    serveMux.HandleFunc("POST /api/login", apiCfg.HandleLogin)
    serveMux.HandleFunc("POST /api/refresh", apiCfg.HandleRefresh)
    serveMux.HandleFunc("POST /api/revoke", apiCfg.HandleRevoke)
    // Webhooks
    serveMux.HandleFunc("POST /api/polka/webhooks", apiCfg.HandlePolkaEvent)
    //============================== ADMIN ==============================
    serveMux.HandleFunc("GET /admin/metrics", apiCfg.HandleMetrics)
    serveMux.HandleFunc("POST /admin/reset", apiCfg.HandleReset)

    server := http.Server { Handler: serveMux, Addr: ":8080" }
    server.ListenAndServe()
}
