// TODO's (apart from the ones littered throughout the code already)
//
//
// TODO
// Break up handlers into separate files based on resource endpoint?
// API is slowly getting large enough to get a good idea of how it should best be broken up.
// Idea:
//      ./internal/handlers
//
//      ./internal/handlers/api
//      ./internal/handlers/api/users: all user-related api actions
//      ./internal/handlers/api/chirps: all chirp-related api actions
//      ./internal/handlers/api/auth: login, revoke refresh token, refresh refresh token, etc
//
// Maybe just keep helper functions (e.g. SendJsonErrorResponse) in main.go?
// Should login actually go under /api/users since it responds with the user?
// Or maybe login and refresh token revocation/refresh should _all_ go into /api/users?

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

// Try to decode request parameters, send an error response on decode failure.
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

func (cfg *ApiConfig) HandleCreateUser(res http.ResponseWriter, req *http.Request) {
    type RequestParameters struct  {
        Email string `json:"email"`
        Password string `json:"password"`
    }
    var reqParams RequestParameters
    if err, errCode := DecodeRequestBodyParameters(&reqParams, res, req); err != nil {
        SendJsonErrorResponse(res, errCode, err.Error())
        return
    }

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

    // TODO pull this out since it's shared between HandleUpdateUser and HandleCreateUser
    // Intentionally omitting hashed_password field
    type ResponseUser struct {
        ID              uuid.UUID   `json:"id"`
        CreatedAt       time.Time   `json:"created_at"`
        UpdatedAt       time.Time   `json:"updated_at"`
        Email           string      `json:"email"`
        IsChirpyRed     bool        `json:"is_chirpy_red"`
    }
    responseUser := ResponseUser {
        ID: user.ID,
        CreatedAt: user.CreatedAt,
        UpdatedAt: user.UpdatedAt,
        Email: user.Email,
        IsChirpyRed: user.IsChirpyRed,
    }

    SendJsonResponse(res, http.StatusCreated, responseUser)
}

func (cfg *ApiConfig) HandleUpdateUser(res http.ResponseWriter, req *http.Request) {
    type RequestParameters struct  {
        Email string `json:"email"`
        Password string `json:"password"`
    }
    var reqParams RequestParameters
    if err, errCode := DecodeRequestBodyParameters(&reqParams, res, req); err != nil {
        SendJsonErrorResponse(res, errCode, err.Error())
        return
    }

    userId, err, errCode := GetAuthenticatedUserId(req.Header, cfg.Secret)
    if err != nil {
        SendJsonErrorResponse(res, errCode, err.Error())
        return
    }

    hashedPassword, err := auth.HashPassword(reqParams.Password)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusInternalServerError, "failed to encrypt password")
        return
    }

    params := database.UpdateUserParams {
        ID: userId,
        Email: reqParams.Email,
        HashedPassword: hashedPassword,
    }
    user, err := cfg.Db.UpdateUser(req.Context(), params)

    // TODO pull this out since it's shared between HandleUpdateUser and HandleCreateUser
    // Intentionally omitting hashed_password field
    type ResponseUser struct {
        ID              uuid.UUID   `json:"id"`
        CreatedAt       time.Time   `json:"created_at"`
        UpdatedAt       time.Time   `json:"updated_at"`
        Email           string      `json:"email"`
        IsChirpyRed     bool        `json:"is_chirpy_red"`
    }
    responseUser := ResponseUser {
        ID: user.ID,
        CreatedAt: user.CreatedAt,
        UpdatedAt: user.UpdatedAt,
        Email: user.Email,
        IsChirpyRed: user.IsChirpyRed,
    }

    SendJsonResponse(res, http.StatusOK, responseUser)
}

func (cfg *ApiConfig) HandleCreateChirp(res http.ResponseWriter, req *http.Request) {
    type RequestParameters struct  { Body string `json:"body"` }
    var reqParams RequestParameters
    if err, errCode := DecodeRequestBodyParameters(&reqParams, res, req); err != nil {
        SendJsonErrorResponse(res, errCode, err.Error())
        return
    }

    userId, err, errCode := GetAuthenticatedUserId(req.Header, cfg.Secret)
    if err != nil {
        SendJsonErrorResponse(res, errCode, err.Error())
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

func (cfg *ApiConfig) HandleDeleteChirp(res http.ResponseWriter, req *http.Request) {
    idStr := req.PathValue("id")
    idUuid, err := uuid.Parse(idStr)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusBadRequest, "invalid uuid")
        return
    }

    authenticatedUserId, err, errCode := GetAuthenticatedUserId(req.Header, cfg.Secret)
    if err != nil {
        SendJsonErrorResponse(res, errCode, err.Error())
        return
    }

    chirp, err := cfg.Db.GetChirp(req.Context(), idUuid)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusNotFound, "chirp not found")
        return
    }
    if chirp.UserID != authenticatedUserId {
        SendJsonErrorResponse(res, http.StatusForbidden, "forbidden")
        return
    }

    if _, err = cfg.Db.DeleteChirp(req.Context(), chirp.ID); err != nil {
        SendJsonResponse(res, http.StatusInternalServerError, "failed to delete chirp")
        return
    }
    res.WriteHeader(http.StatusNoContent)
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
    }
    var reqParams RequestParameters
    if err, errCode := DecodeRequestBodyParameters(&reqParams, res, req); err != nil {
        SendJsonErrorResponse(res, errCode, err.Error())
        return
    }

    user, err := cfg.Db.GetUserByEmail(req.Context(), reqParams.Email)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusNotFound, "no user with the provided email exists")
        return
    }

    if err := auth.CheckPasswordHash(reqParams.Password, user.HashedPassword); err != nil {
        SendJsonErrorResponse(res, http.StatusUnauthorized, "incorrect email or password")
        return
    }

    accessToken, err := auth.MakeJWT(user.ID, cfg.Secret, time.Hour)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusInternalServerError, "failed to make access token")
        return
    }

    refreshToken, err := auth.MakeRefreshToken()
    if err != nil {
        SendJsonErrorResponse(res, http.StatusInternalServerError, "failed to make refresh token")
        return
    }
    now := time.Now()
    refreshTokenExpiry := now.Add(60 * 24 * time.Hour)
    dbParams := database.CreateRefreshTokenParams {
        Token: refreshToken,
        UserID: user.ID,
        ExpiresAt: refreshTokenExpiry,
    }
    if _, err := cfg.Db.CreateRefreshToken(req.Context(), dbParams); err != nil {
        SendJsonErrorResponse(res, http.StatusInternalServerError, "failed to add refresh token to database")
        return
    }

    type ResponseUser struct {
        ID              uuid.UUID   `json:"id"`
        CreatedAt       time.Time   `json:"created_at"`
        UpdatedAt       time.Time   `json:"updated_at"`
        Email           string      `json:"email"`
        IsChirpyRed     bool        `json:"is_chirpy_red"`
        Token           string      `json:"token"`
        RefreshToken    string      `json:"refresh_token"`
    }
    responseUser := ResponseUser {
        ID: user.ID,
        CreatedAt: user.CreatedAt,
        UpdatedAt: user.UpdatedAt,
        Email: user.Email,
        IsChirpyRed: user.IsChirpyRed,
        Token: accessToken,
        RefreshToken: refreshToken,
    }
    SendJsonResponse(res, http.StatusOK, responseUser)
}

// Send a new access token given a valid (non-expired and non-revoked) refresh token
func (cfg *ApiConfig) HandleRefresh(res http.ResponseWriter, req *http.Request) {
    bearerToken, err := auth.GetBearerToken(req.Header)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusBadRequest, err.Error())
        return
    }

    refreshToken, err := cfg.Db.GetRefreshToken(req.Context(), bearerToken)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusUnauthorized, "refresh token does not exist")
        return
    }
    now := time.Now()
    if now.After(refreshToken.ExpiresAt) {
        SendJsonErrorResponse(res, http.StatusUnauthorized, "refresh token expired")
        return
    }
    if refreshToken.RevokedAt.Valid && now.After(refreshToken.RevokedAt.Time) {
        SendJsonErrorResponse(res, http.StatusUnauthorized, "refresh token revoked")
        return
    }

    user, err := cfg.Db.GetUserFromRefreshToken(req.Context(), refreshToken.Token)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusInternalServerError, "failed to get user from refresh token")
        return
    }

    type ResponseBody struct { Token string `json:"token"` }
    accessToken, err := auth.MakeJWT(user.ID, cfg.Secret, time.Hour)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusInternalServerError, "failed to create access token")
        return
    }
    SendJsonResponse(res, http.StatusOK, ResponseBody { Token: accessToken })
}

func (cfg *ApiConfig) HandleRevoke(res http.ResponseWriter, req *http.Request) {
    bearerToken, err := auth.GetBearerToken(req.Header)
    if err != nil {
        SendJsonErrorResponse(res, http.StatusBadRequest, err.Error())
        return
    }

    _, err = cfg.Db.RevokeRefreshToken(req.Context(), bearerToken)
    if err != nil {
        // TODO figure out what the response status should actually be
        SendJsonErrorResponse(res, http.StatusInternalServerError, "failed to revoke refresh token")
        return
    }

    res.WriteHeader(http.StatusNoContent)
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
    // Chirps
    serveMux.HandleFunc("GET /api/chirps", apiCfg.HandleGetChirps)
    serveMux.HandleFunc("GET /api/chirps/{id}", apiCfg.HandleGetChirp)
    serveMux.HandleFunc("POST /api/chirps", apiCfg.HandleCreateChirp)
    serveMux.HandleFunc("DELETE /api/chirps/{id}", apiCfg.HandleDeleteChirp)
    // Users
    serveMux.HandleFunc("POST /api/users", apiCfg.HandleCreateUser)
    serveMux.HandleFunc("PUT /api/users", apiCfg.HandleUpdateUser)
    // Auth
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
