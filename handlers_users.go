package main

import (
    "net/http"
    "time"
    "fmt"

    "github.com/google/uuid"

    "github.com/vedaRadev/chirpy-boot.dev/internal/auth"
    "github.com/vedaRadev/chirpy-boot.dev/internal/database"
)

type ResponseUser struct {
    ID              uuid.UUID   `json:"id"`
    CreatedAt       time.Time   `json:"created_at"`
    UpdatedAt       time.Time   `json:"updated_at"`
    Email           string      `json:"email"`
    IsChirpyRed     bool        `json:"is_chirpy_red"`
    // TODO move Token and RefreshToken to their own type and embed responseuser OR make them
    // nullable strings (*string)?
    Token           string      `json:"token"`
    RefreshToken    string      `json:"refresh_token"`
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
    if err != nil {
        SendJsonErrorResponse(res, http.StatusInternalServerError, "failed to update user")
        return
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
