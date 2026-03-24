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
		// 1. Check API Key in header or query param
		key := r.Header.Get("X-API-KEY")
		if key == "" {
			key = r.URL.Query().Get("api_key")
		}

		if key == apiKey {
			next(w, r)
			return
		}

		// 2. Check JWT in header or query param
		tokenString := ""
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			bearerToken := strings.Split(authHeader, " ")
			if len(bearerToken) == 2 && strings.ToLower(bearerToken[0]) == "bearer" {
				tokenString = bearerToken[1]
			}
		} else {
			tokenString = r.URL.Query().Get("token")
		}

		if tokenString == "" {
			log.Warn().Msg("missing authentication (header or query param)")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

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

