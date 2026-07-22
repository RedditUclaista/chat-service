package http

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gocql/gocql"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v5"
)

type CustomClaims struct {
	jwt.RegisteredClaims
	UserID    string
	Email     string
	Role      string
	SessionID string
}

func JWTAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		authHeader := c.Request().Header.Get("Authorization")
		if authHeader == "" {
			return echo.NewHTTPError(http.StatusUnauthorized, "autorizacion requerida")
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return echo.NewHTTPError(http.StatusUnauthorized, "formato de autorizacion invalido")
		}

		secret := os.Getenv("JWT_SECRET")
		if secret == "" {
			secret = "esto es una key re segura"
		}

		token, err := jwt.ParseWithClaims(parts[1], &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("metodo de firma inesperado: %v", token.Header["alg"])
			}
			return []byte(secret), nil
		})

		if err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "token invalido: "+err.Error())
		}

		claims, ok := token.Claims.(*CustomClaims)
		if !ok || !token.Valid {
			return echo.NewHTTPError(http.StatusUnauthorized, "claims del token invalidos")
		}

		userID, err := gocql.ParseUUID(claims.UserID)
		if err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "user_id invalido en el token")
		}

		c.Set("user_id", userID)
		c.Set("user_email", claims.Email)
		c.Set("user_role", claims.Role)

		return next(c)
	}
}
