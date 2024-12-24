// TODO's (apart from the ones littered throughout the code already)
//
// Helper function for sending simple json error responses?
// I don't really see much benefit to this yet since it really doesn't cut down on code length or
// make anything more concise. It _maybe_ could save me from typos when creating the json body but
// it's literally one field and I just don't see it being an issue.
// PRO: If I want to change the field(s) we're sending in the JSON I just have to update it in one place.
//
// Deal with potential errors when marshalling response body data into JSON.
//
// Helper function for sending JSON responses?
//
// Break up handlers into separate files based on resource endpoint?
// I don't think the API is really large enough to warrant this yet.
//
// Helper function to decode request parameters (maybe also send error if failure)

// FIXME's
// In most API handlers I'm assuming that the marshalling of structs into JSON never fails but this
// might be a bad assumption.

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
    type RequestParameters struct  {
        Email string `json:"email"`
        Password string `json:"password"`
    }

    res.Header().Set("Content-Type", "application/json")

    var reqParams RequestParameters
    if err := json.NewDecoder(req.Body).Decode(&reqParams); err != nil {
        res.WriteHeader(http.StatusInternalServerError)
        res.Write([]byte(`{"error":"Failed to decode request json body"}`))
        return
    }

    hashedPassword, err := auth.HashPassword(reqParams.Password)
    if err != nil {
        res.WriteHeader(http.StatusInternalServerError)
        res.Write([]byte(`{"error":"Failed to encrypt password"}`))
        return
    }

    params := database.CreateUserParams {
        Email: reqParams.Email,
        HashedPassword: hashedPassword,
    }
    user, err := cfg.Db.CreateUser(req.Context(), params)
    if err != nil {
        res.WriteHeader(http.StatusInternalServerError)
        res.Write([]byte(`{"error":"Failed to create user"}`))
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

    res.WriteHeader(http.StatusCreated)
    resBody, _ := json.Marshal(responseUser)
    res.Write(resBody)
}

func (cfg *ApiConfig) HandleCreateChirp(res http.ResponseWriter, req *http.Request) {
    type RequestParameters struct  {
        Body string `json:"body"`
        UserID uuid.UUID `json:"user_id"`
    }

    res.Header().Set("Content-Type", "application/json")

    var reqParams RequestParameters
    if err := json.NewDecoder(req.Body).Decode(&reqParams); err != nil {
        res.WriteHeader(http.StatusInternalServerError)
        res.Write([]byte(`{"error":"Failed to decode request json body"}`))
        return
    }

    // TODO pull out into helper, maybe just have it return a boolean
    const MAX_CHIRP_LEN int = 140
    if len(reqParams.Body) > MAX_CHIRP_LEN {
        res.WriteHeader(http.StatusBadRequest)
        res.Write([]byte(`{"error":"Chirp is too long"}`))
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

    params := database.CreateChirpParams { Body: cleaned, UserID: reqParams.UserID }
    chirp, err := cfg.Db.CreateChirp(req.Context(), params)
    if err != nil {
        res.WriteHeader(http.StatusInternalServerError)
        res.Write([]byte(`{"error":"Failed to create chirp in database"}`))
        fmt.Printf(`Failed to create chirp in db: user %v, chirp "%v"\n`, reqParams.UserID, cleaned)
        return
    }

    res.WriteHeader(http.StatusCreated)
    resBody, _ := json.Marshal(chirp)
    res.Write(resBody)
}

func (cfg *ApiConfig) HandleGetChirps(res http.ResponseWriter, req *http.Request) {
    // NOTE no params for now

    res.Header().Set("Content-Type", "application/json")

    chirps, err := cfg.Db.GetChirps(req.Context())
    if err != nil {
        res.WriteHeader(http.StatusInternalServerError)
        res.Write([]byte(`{"error":"Failed to retrieve chirps from database"}`))
        fmt.Printf("Failed to retrieve chirps from databse: %v\n", err)
        return
    }

    res.WriteHeader(http.StatusOK)
    resBody, _ := json.Marshal(chirps)
    res.Write(resBody)
}

func (cfg *ApiConfig) HandleGetChirp(res http.ResponseWriter, req *http.Request) {
    res.Header().Set("Content-Type", "application/json")

    idStr := req.PathValue("id")
    idUuid, err := uuid.Parse(idStr)
    if err != nil {
        res.WriteHeader(http.StatusBadRequest)
        res.Write([]byte(`{"error":"Invalid uuid"}`))
        return
    }

    chirp, err := cfg.Db.GetChirp(req.Context(), idUuid)
    if err != nil {
        res.WriteHeader(http.StatusNotFound)
        res.Write([]byte(`{"error":"Failed to get chirp"}`))
        return
    }

    res.WriteHeader(http.StatusOK)
    resBody, _ := json.Marshal(chirp)
    res.Write(resBody)
}

func (cfg *ApiConfig) HandleLogin(res http.ResponseWriter, req *http.Request) {
    type RequestParameters struct  {
        Email string `json:"email"`
        Password string `json:"password"`
    }

    var reqParams RequestParameters
    if err := json.NewDecoder(req.Body).Decode(&reqParams); err != nil {
        res.WriteHeader(http.StatusInternalServerError)
        res.Write([]byte(`{"error":"Failed to decode request json body"}`))
        return
    }

    user, err := cfg.Db.GetUserByEmail(req.Context(), reqParams.Email)
    if err != nil {
        res.WriteHeader(http.StatusNotFound)
        res.Write([]byte(`{"error":"User with the provided email not found"}`))
        return
    }

    if err := auth.CheckPasswordHash(reqParams.Password, user.HashedPassword); err != nil {
        res.WriteHeader(http.StatusUnauthorized)
        res.Write([]byte(`{"error":"Incorrect email or password"}`))
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

    res.WriteHeader(http.StatusOK)
    resBody, _ := json.Marshal(responseUser)
    res.Write(resBody)
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
    apiCfg := ApiConfig { Platform: platform, Db: dbQueries }

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
