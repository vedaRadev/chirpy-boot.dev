package auth

import (
    "time"
    "net/http"
    "fmt"
    "strings"
    "encoding/hex"
    "crypto/rand"
    "golang.org/x/crypto/bcrypt"
    "github.com/google/uuid"
    "github.com/golang-jwt/jwt/v5"
)

func HashPassword(password string) (string, error) {
    hashed, err := bcrypt.GenerateFromPassword([]byte(password), 0)
    if err != nil { return "", err }
    return string(hashed), nil
}

func CheckPasswordHash(password, hash string) error {
    return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func MakeJWT(userId uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
    now := time.Now()
    claims := jwt.RegisteredClaims {
        Issuer: "chirpy",
        IssuedAt: jwt.NewNumericDate(now),
        ExpiresAt: jwt.NewNumericDate(now.Add(expiresIn)),
        Subject: userId.String(),
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    signedString, err := token.SignedString([]byte(tokenSecret))
    if err != nil { return "", err }
    return signedString, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
    var result uuid.UUID

    token, err := jwt.ParseWithClaims(
        tokenString,
        &jwt.RegisteredClaims {},
        func (token *jwt.Token) (any, error) { return []byte(tokenSecret), nil },
    )
    if err != nil { return result, err }

    id, err := token.Claims.GetSubject()
    if err != nil { return result, err }

    result, err = uuid.Parse(id)
    if err != nil { return result, err }

    return result, nil
}

func getAuthHeaderValue(header http.Header, fieldName string) (string, error) {
    authHeader := header.Get("Authorization")
    if authHeader == "" { return "", fmt.Errorf("authorization header not found") }
    if !strings.Contains(authHeader, fieldName) { return "", fmt.Errorf("invalid auth header format") }

    fields := strings.Fields(authHeader)
    if len(fields) != 2 { return "", fmt.Errorf("invalid auth header format") }
    return fields[1], nil
}

func GetBearerToken(header http.Header) (string, error) {
    return getAuthHeaderValue(header, "Bearer")
}

func GetApiKey(header http.Header) (string, error) {
    return getAuthHeaderValue(header, "ApiKey")
}

func MakeRefreshToken() (string, error) {
    bytes := make([]byte, 32)
    if _, err := rand.Read(bytes); err != nil { return "", err }
    return hex.EncodeToString(bytes), nil
}
