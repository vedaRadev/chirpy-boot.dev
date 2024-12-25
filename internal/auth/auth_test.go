package auth

import (
    "testing"
    "time"
    "net/http"
    "github.com/google/uuid"
)

func TestJWTCreationAndValidation(t *testing.T) {
    expectedId := uuid.New()
    secret := "test secret"
    expiry := 5 * time.Second

    token, err := MakeJWT(expectedId, secret, expiry)
    if err != nil {
        t.Errorf("Token creation failed but shouldn't have: %v\n", err.Error())
        t.FailNow()
    }

    resultId, err := ValidateJWT(token, secret)
    if err != nil {
        t.Errorf("Token validation failed but shouldn't have: %v\n", err.Error())
        t.FailNow()
    }

    if resultId != expectedId {
        t.Errorf("decoded id doesn't match the expected id! actual: %s, expected: %s\n", resultId, expectedId)
        t.FailNow()
    }
}

func TestJWTExpires(t *testing.T) {
    expectedId := uuid.New()
    secret := "test secret"
    expiry := 1 * time.Second

    token, err := MakeJWT(expectedId, secret, expiry)
    if err != nil {
        t.Errorf("Token creation failed but shouldn't have: %v\n", err.Error())
        t.FailNow()
    }

    time.Sleep(2 * time.Second)

    _, err = ValidateJWT(token, secret)
    if err == nil {
        t.Error("Token failed to expire")
        t.FailNow()
    }
}

func TestJWTValidationFailsWithWrongSecret(t *testing.T) {
    id := uuid.New()
    goodSecret := "good token"
    expiry := 5 * time.Second

    badToken, err := MakeJWT(id, "bad token", expiry)
    if err != nil {
        t.Errorf("Token creation failed but shouldn't have: %v\n", err.Error())
        t.FailNow()
    }

    _, err = ValidateJWT(badToken, goodSecret)
    if err == nil {
        t.Error("Validation should have failed!")
        t.FailNow()
    }
}

func TestGetBearerToken(t *testing.T) {
    testCases := []struct {
        in http.Header
        expectedOut string
        shouldError bool
    }{
        {
            in: http.Header { "Authorization": { "Bearer testauth" } },
            expectedOut: "testauth",
            shouldError: false,
        },
        {
            in: http.Header { "Authorization": { "testauth" } },
            expectedOut: "",
            shouldError: true,
        },
        {
            in: http.Header { "Authorization": { "Bearer" } },
            expectedOut: "",
            shouldError: true,
        },
        {
            in: http.Header { "Authorization": { "" } },
            expectedOut: "",
            shouldError: true,
        },
        {
            in: http.Header {},
            expectedOut: "",
            shouldError: true,
        },
    }

    for i := range testCases {
        testCase := testCases[i]
        out, err := GetBearerToken(testCase.in)

        if !testCase.shouldError && err != nil {
            t.Errorf("Test case %v: case errored but was not expected to\n", i)
            continue
        }

        if testCase.shouldError && err == nil {
            t.Errorf("Test case %v: case did not error but was expected to\n", i)
            continue
        }

        if testCase.expectedOut != out {
            t.Errorf(
                "Test case %v: unexpected result (actual \"%v\" != expected \"%v\")\n",
                i,
                out,
                testCase.expectedOut,
            )
            continue
        }
    }
}
