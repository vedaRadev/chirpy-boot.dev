# Chirpy
A guided project from boot.dev for learning http servers in Go.

### Requirements
- [postgres](https://www.postgresql.org/)
- [go](https://go.dev/)
- [goose](https://github.com/pressly/goose) for running database migrations
- [sqlc](https://sqlc.dev/) for generating database models, queries, etc.

### Installing from Source
1. Clone the repository.
2. Run `go install` in the project root. This will create an executable that can be run with `chirpy-boot.dev`.

### Setup
The Chirpy server expects a `.env` file in the project root containing the following values:
```
DB_URL="..."    ; postgres chirpy dbl url, sslmode=disable
PLATFORM="dev"  ; dev = enable admin endpoints
SECRET="..."    ; anything, used for JWT creation/validation
POLKA_KEY="..." ; an imaginary API key for simulating a webhook event handler
```

Create the `chirpy` database in postgres:
```SQL
CREATE DATABASE chirpy
```

Run the goose migrations inside of `./sql/schema`: `goose postgres "chirpy_db_url_here" up`.

### Running
`go run .` or build then run the compiled executable to start the server on port `8080`.

## Endpoints
TODO documentation. I might not get around to actually documenting these endpoints because this was created from a guided project and isn't really all that impressive.
