// TODO's (apart from the ones littered throughout the code already)
//
// Break up handlers into separate files based on resource endpoint?
// I don't think the API is really large enough to warrant this yet.

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
    "time"

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

// Try to decode request parameters, send an error response on decode failure.
func DecodeRequestBodyParameters[T any](reqParams *T, res http.ResponseWriter, req *http.Request) bool {
    if err := json.NewDecoder(req.Body).Decode(reqParams); err != nil {
        SendJsonErrorResponse(res, http.StatusInternalServerError, "failed to decode request body")
        return false
    }

    return true
}

type ApiConfig struct  {
    FileServerHits atomic.Int32
    Platform string
    Secret string
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

func (cfg *ApiConfig) HandleCreateUser(res http.ResponseWriter, req *http.Request) {
    type RequestParameters struct  {
        Email string `json:"email"`
        Password string `json:"password"`
    }
    var reqParams RequestParameters
    if success := DecodeRequestBodyParameters(&reqParams, res, req); !success { return }

    hashedPassword, err := auth.HashPassword(reqParams.Password)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusInternalServerError, "failed to encrypt password")
        return
    }

    params := database.CreateUserParams {
        Email: reqParams.Email,
        HashedPassword: hashedPassword,
    }
    user, err := cfg.Db.CreateUser(req.Context(), params)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusInternalServerError, "failed to create user")
        fmt.Printf("Failed to create user %v: %v\n", reqParams.Email, err.Error())
        return
    }

    // Intentionally omitting hashed_password field
    type ResponseUser struct {
        ID             uuid.UUID `json:"id"`
        CreatedAt      time.Time `json:"created_at"`
        UpdatedAt      time.Time `json:"updated_at"`
        Email          string    `json:"email"`
    }
    responseUser := ResponseUser {
        ID: user.ID,
        CreatedAt: user.CreatedAt,
        UpdatedAt: user.UpdatedAt,
        Email: user.Email,
    }

    SendJsonResponse(res, http.StatusCreated, responseUser)
}

func (cfg *ApiConfig) HandleCreateChirp(res http.ResponseWriter, req *http.Request) {
    type RequestParameters struct  { Body string `json:"body"` }
    var reqParams RequestParameters
    if success := DecodeRequestBodyParameters(&reqParams, res, req); !success { return }

    bearerToken, err := auth.GetBearerToken(req.Header)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusBadRequest, err.Error())
        return
    }

    userId, err := auth.ValidateJWT(bearerToken, cfg.Secret)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusUnauthorized, "failed to authorize jwt")
        fmt.Printf("failed to authorize jwt: %v\n", err.Error())
        return
    }

    // TODO pull out into helper, maybe just have it return a boolean
    const MAX_CHIRP_LEN int = 140
    if len(reqParams.Body) > MAX_CHIRP_LEN {
        SendJsonErrorResponse(res, http.StatusBadRequest, "chirp is too long")
        return
    }

    // TODO pull out into helper, return the cleaned chirp body as a string
    // Filter profanity
    words := strings.Split(reqParams.Body, " ")
    for i := range words {
        lower := strings.ToLower(words[i])
        if lower == "kerfuffle" || lower == "sharbert" || lower == "fornax" {
            words[i] = "****"
        }
    }
    cleaned := strings.Join(words, " ")

    params := database.CreateChirpParams { Body: cleaned, UserID: userId }
    chirp, err := cfg.Db.CreateChirp(req.Context(), params)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusInternalServerError, "failed to create chirp")
        fmt.Printf(`Failed to create chirp in db: user %v, chirp "%v"\n`, userId, cleaned)
        return
    }

    SendJsonResponse(res, http.StatusCreated, chirp)
}

func (cfg *ApiConfig) HandleGetChirps(res http.ResponseWriter, req *http.Request) {
    chirps, err := cfg.Db.GetChirps(req.Context())
    if err != nil {
        SendJsonErrorResponse(res, http.StatusInternalServerError, "failed to get chirps")
        fmt.Printf("Failed to retrieve chirps from databse: %v\n", err)
        return
    }

    SendJsonResponse(res, http.StatusOK, chirps)
}

func (cfg *ApiConfig) HandleGetChirp(res http.ResponseWriter, req *http.Request) {
    idStr := req.PathValue("id")
    idUuid, err := uuid.Parse(idStr)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusBadRequest, "invalid uuid")
        return
    }

    chirp, err := cfg.Db.GetChirp(req.Context(), idUuid)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusNotFound, "failed to get chirp")
        return
    }

    SendJsonResponse(res, http.StatusOK, chirp)
}

func (cfg *ApiConfig) HandleLogin(res http.ResponseWriter, req *http.Request) {
    type RequestParameters struct  {
        Email string `json:"email"`
        Password string `json:"password"`
        ExpiresInSeconds *int `json:"expires_in_seconds"`
    }
    var reqParams RequestParameters
    if success := DecodeRequestBodyParameters(&reqParams, res, req); !success { return }

    user, err := cfg.Db.GetUserByEmail(req.Context(), reqParams.Email)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusNotFound, "no user with the provided email exists")
        return
    }

    if err := auth.CheckPasswordHash(reqParams.Password, user.HashedPassword); err != nil {
        SendJsonErrorResponse(res, http.StatusUnauthorized, "incorrect email or password")
        return
    }

    expiresIn := time.Hour
    if reqParams.ExpiresInSeconds != nil {
        clientDefinedDuration := time.Duration(*reqParams.ExpiresInSeconds) * time.Second
        if clientDefinedDuration > 0 && clientDefinedDuration < time.Hour {
            expiresIn = clientDefinedDuration
        }
    }

    token, err := auth.MakeJWT(user.ID, cfg.Secret, expiresIn)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusInternalServerError, "failed to make jwt")
        return
    }

    // Intentionally omitting hashed_password field
    type ResponseUser struct {
        ID          uuid.UUID `json:"id"`
        CreatedAt   time.Time `json:"created_at"`
        UpdatedAt   time.Time `json:"updated_at"`
        Email       string    `json:"email"`
        Token       string    `json:"token"`
    }
    responseUser := ResponseUser {
        ID: user.ID,
        CreatedAt: user.CreatedAt,
        UpdatedAt: user.UpdatedAt,
        Email: user.Email,
        Token: token,
    }

    SendJsonResponse(res, http.StatusOK, responseUser)
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
    apiCfg := ApiConfig { Platform: platform, Secret: secret, Db: dbQueries }

    serveMux := http.NewServeMux()
    //============================== APP ==============================
    serveMux.Handle("/app/", apiCfg.MiddlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir("site")))))
    //============================== API ==============================
    serveMux.HandleFunc("GET /api/healthz", func(res http.ResponseWriter, req *http.Request) {
        res.WriteHeader(http.StatusOK)
        res.Header().Add("Content-Type", "text/plain; charset=utf-8")
        res.Write([]byte("OK"))
    })
    serveMux.HandleFunc("GET /api/chirps", apiCfg.HandleGetChirps)
    serveMux.HandleFunc("GET /api/chirps/{id}", apiCfg.HandleGetChirp)
    serveMux.HandleFunc("POST /api/chirps", apiCfg.HandleCreateChirp)
    serveMux.HandleFunc("POST /api/users", apiCfg.HandleCreateUser)
    serveMux.HandleFunc("POST /api/login", apiCfg.HandleLogin)
    //============================== ADMIN ==============================
    serveMux.HandleFunc("GET /admin/metrics", apiCfg.HandleMetrics)
    serveMux.HandleFunc("POST /admin/reset", apiCfg.HandleReset)

    server := http.Server { Handler: serveMux, Addr: ":8080" }
    server.ListenAndServe()
}
