package auth

import (
    "testing"
    "net/http"
)

func TestMakeJWT(t *testing.T) {
    // TODO
}

func TestValidateJWT(t *testing.T) {
    // TODO
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
