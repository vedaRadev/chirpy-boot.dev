package main

import (
    "strings"
    "net/http"
    "fmt"
    "github.com/google/uuid"
    "github.com/vedaRadev/chirpy-boot.dev/internal/database"
)

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

func (cfg *ApiConfig) HandleGetChirps(res http.ResponseWriter, req *http.Request) {
    var chirps []database.Chirp
    var err error

    queryValues := req.URL.Query()

    authorId := queryValues.Get("author_id")
    sortOrder := strings.ToUpper(queryValues.Get("sort"))
    if sortOrder == "" || (sortOrder != "ASC" && sortOrder != "DESC") { sortOrder = "ASC" }

    if authorId == "" {
        getChirps := cfg.Db.GetChirps
        if sortOrder == "DESC" { getChirps = cfg.Db.GetChirpsDesc }
        chirps, err = getChirps(req.Context())
    } else {
        userId, err := uuid.Parse(authorId)
        if err != nil {
            SendJsonErrorResponse(res, http.StatusBadRequest, "invalid author id")
            return
        }

        getUserChirps := cfg.Db.GetUserChirps
        if sortOrder == "DESC" { getUserChirps = cfg.Db.GetUserChirpsDesc }
        chirps, err = getUserChirps(req.Context(), userId)
    }
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
