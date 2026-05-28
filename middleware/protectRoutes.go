package middleware

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Public to allow finer configuration
var ProtectRoutesSettings = struct {
	// "JWT_SECRET"
	JwtSecretEnvKey string

	// "auth_token"
	CookieName string

	// "X-User-ID"
	UserIDHeader string

	// Default 24 hours
	TokenExpiry time.Duration

	// Default false, set to true if using HTTPS
	SecureCookie bool
}{
	JwtSecretEnvKey: "JWT_SECRET",
	CookieName:      "auth_token",
	UserIDHeader:    "X-User-ID",
	TokenExpiry:     time.Hour * 24,
	SecureCookie:    false,
}

type ProtectRoutesConfig struct {
	// If empty, protect everything
	SecuredRoutes string

	// Whitelisted routes, always available
	PermittedRoutes string

	// Never protected
	Redirect string
}

func (c *ProtectRoutesConfig) isProtected(path string) bool {
	// Never protect the redirect path
	if c.Redirect != "" && path == c.Redirect {
		return false
	}

	// Check whitelist first
	if c.PermittedRoutes != "" && strings.HasPrefix(path, c.PermittedRoutes) {
		return false
	}

	// If SecuredRoutes is empty, auth all paths.
	if c.SecuredRoutes == "" {
		return true
	}

	if strings.HasPrefix(path, c.SecuredRoutes) {
		return true
	}

	return false
}

func (c *ProtectRoutesConfig) noCredentials(w http.ResponseWriter, r *http.Request) {
	if c.Redirect != "" {
		http.Redirect(w, r, c.Redirect, http.StatusFound)
		return
	}

	http.Error(w, "Unauthorized", 401)
}

func (c *ProtectRoutesConfig) invalidCredentials(w http.ResponseWriter, r *http.Request) {
	if c.Redirect != "" {
		http.Redirect(w, r, c.Redirect, http.StatusFound)
		return
	}

	http.Error(w, "Unauthorized", 403)
}

// Panics if JWT_SECRET environment variable not set
func ProtectRoutes(config ProtectRoutesConfig) func(http.Handler) http.Handler {
	secret, ok := os.LookupEnv(ProtectRoutesSettings.JwtSecretEnvKey)

	if !ok {
		err := fmt.Errorf("%v environment variable not set", ProtectRoutesSettings.JwtSecretEnvKey)
		panic(err)
	}

	f := func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			r.Header.Set(ProtectRoutesSettings.UserIDHeader, "") // Clear to avoid possible fraud

			path := r.URL.Path

			if !config.isProtected(path) {
				next.ServeHTTP(w, r)
				return
			}

			cookie, err := r.Cookie(ProtectRoutesSettings.CookieName)

			if err != nil {
				config.noCredentials(w, r)
				return
			}

			tokenString := cookie.Value

			subject, err := validateJWT(tokenString, secret)

			if err != nil {
				config.invalidCredentials(w, r)
				return
			}

			r.Header.Set(ProtectRoutesSettings.UserIDHeader, subject)

			next.ServeHTTP(w, r)
		}

		return http.HandlerFunc(fn)
	}

	return f
}

func validateJWT(tokenString, secret string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		return []byte(secret), nil
	})

	if err != nil {
		return "", err
	}

	if !token.Valid {
		return "", fmt.Errorf("invalid token")
	}

	subject, err := token.Claims.GetSubject()

	return subject, err
}

// Grants an auth cookie
func Authorize(subject string, w http.ResponseWriter) error {
	secret, ok := os.LookupEnv(ProtectRoutesSettings.JwtSecretEnvKey)

	if !ok {
		err := fmt.Errorf("%v environment variable not set", ProtectRoutesSettings.JwtSecretEnvKey)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return err
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": subject,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(ProtectRoutesSettings.TokenExpiry).Unix(),
	})

	tokenString, err := token.SignedString([]byte(secret))

	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     ProtectRoutesSettings.CookieName,
		Value:    tokenString,
		HttpOnly: true,
		Secure:   ProtectRoutesSettings.SecureCookie,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
		MaxAge:   int(ProtectRoutesSettings.TokenExpiry.Seconds()),
	})

	return nil
}

// Logs the user out
func Deauthorize(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     ProtectRoutesSettings.CookieName,
		Value:    "",
		HttpOnly: true,
		Secure:   ProtectRoutesSettings.SecureCookie,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
		MaxAge:   -1,
	})
}
