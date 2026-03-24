package main

import (
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"
)

var (
	jwtSecret = []byte(os.Getenv("JWT_SECRET"))
	apiKey    = os.Getenv("API_KEY")
)

func init() {
	if len(jwtSecret) == 0 {
		jwtSecret = []byte("default-secret")
	}
	if apiKey == "" {
		apiKey = "default-api-key"
	}
}

// AuthMiddleware handles both JWT and API Key authentication
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check API Key first (optional)
		key := r.Header.Get("X-API-KEY")
		if key == apiKey {
			next(w, r)
			return
		}

		// Check JWT
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			log.Warn().Msg("missing authorization header")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		bearerToken := strings.Split(authHeader, " ")
		if len(bearerToken) != 2 || strings.ToLower(bearerToken[0]) != "bearer" {
			log.Warn().Str("header", authHeader).Msg("invalid authorization header format")
			http.Error(w, "invalid token format", http.StatusUnauthorized)
			return
		}

		tokenString := bearerToken[1]
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			log.Warn().Err(err).Msg("invalid jwt token")
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}
