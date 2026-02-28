package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// responseWriter wraps http.ResponseWriter to capture status code and bytes written.
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// recoveryMiddleware catches panics and returns 500.
func recoveryMiddleware(logger *common.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error().
						Str("panic", fmt.Sprintf("%v", rec)).
						Str("path", r.URL.Path).
						Msg("Panic recovered in HTTP handler")
					WriteError(w, http.StatusInternalServerError, "Internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// corsMiddleware adds CORS headers for future web UI.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID, X-Correlation-ID, X-Vire-Portfolios, X-Vire-Display-Currency, X-Vire-Navexa-Key, X-Vire-User-ID, X-Vire-Service-ID")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// correlationIDMiddleware extracts or generates a correlation ID.
func correlationIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Request-ID")
		if corrID == "" {
			corrID = r.Header.Get("X-Correlation-ID")
		}
		if corrID == "" {
			corrID = uuid.New().String()[:8]
		}
		w.Header().Set("X-Correlation-ID", corrID)
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs HTTP requests.
func loggingMiddleware(logger *common.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(rw, r)

			dur := time.Since(start)
			corrID := w.Header().Get("X-Correlation-ID")

			event := logger.Trace()
			if rw.statusCode >= 500 {
				event = logger.Error()
			} else if rw.statusCode >= 400 {
				event = logger.Info()
			}

			event.
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Str("query", r.URL.RawQuery).
				Int("status", rw.statusCode).
				Int("bytes", rw.bytesWritten).
				Dur("duration", dur).
				Str("correlation_id", corrID).
				Msg("HTTP request")
		})
	}
}

// bearerTokenMiddleware checks for an Authorization: Bearer header and,
// if present, validates the JWT and populates UserContext from the token claims.
// If no Authorization header is present, the request passes through to the
// next middleware (existing X-Vire-* header resolution).
//
// When sliding expiry is enabled and the token is >50% through its lifetime,
// a fresh access token is returned in the X-New-Access-Token response header.
func bearerTokenMiddleware(config *common.Config, store interfaces.InternalStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				next.ServeHTTP(w, r)
				return
			}

			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			_, claims, err := validateJWT(tokenString, []byte(config.Auth.JWTSecret))
			if err != nil {
				writeBearerChallenge(w, config, "invalid_token", "invalid or expired token")
				return
			}

			sub, _ := claims["sub"].(string)
			if sub == "" {
				writeBearerChallenge(w, config, "invalid_token", "invalid token claims")
				return
			}

			// Load user from store
			user, err := store.GetUser(r.Context(), sub)
			if err != nil {
				writeBearerChallenge(w, config, "invalid_token", "user not found")
				return
			}

			uc := &common.UserContext{
				UserID: user.UserID,
				Role:   user.Role,
			}

			// Resolve preferences from KV
			if kvs, err := store.ListUserKV(r.Context(), user.UserID); err == nil {
				for _, kv := range kvs {
					switch kv.Key {
					case "navexa_key":
						uc.NavexaAPIKey = kv.Value
					case "display_currency":
						uc.DisplayCurrency = kv.Value
					case "portfolios":
						parts := strings.Split(kv.Value, ",")
						for i := range parts {
							parts[i] = strings.TrimSpace(parts[i])
						}
						uc.Portfolios = parts
					}
				}
			}

			r = r.WithContext(common.WithUserContext(r.Context(), uc))

			// Sliding expiry: if enabled and token is >50% expired, issue a fresh token
			if config.Auth.OAuth2.GetSlidingExpiry() {
				if _, ok := shouldRefreshToken(claims, config); ok {
					// Copy relevant claims for the new token
					clientID, _ := claims["client_id"].(string)
					scope, _ := claims["scope"].(string)
					if scope == "" {
						scope = "vire"
					}
					newTokenString, err := signAccessToken(user, clientID, scope, config)
					if err == nil {
						w.Header().Set("X-New-Access-Token", newTokenString)
						w.Header().Set("X-New-Token-Expires-In", fmt.Sprintf("%d", int(config.Auth.OAuth2.GetAccessTokenExpiry().Seconds())))
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// writeBearerChallenge writes a 401 response with RFC 9728 WWW-Authenticate header.
func writeBearerChallenge(w http.ResponseWriter, config *common.Config, errorCode, description string) {
	issuer := config.Auth.OAuth2.Issuer
	if issuer != "" {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(
			`Bearer error="%s", error_description="%s", resource_metadata="%s/.well-known/oauth-protected-resource"`,
			errorCode, description, issuer))
	} else {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	WriteError(w, http.StatusUnauthorized, description)
}

// shouldRefreshToken checks if the token should be refreshed (>50% through lifetime).
// Returns (newClaims, shouldRefresh) where newClaims contains updated iat/exp.
func shouldRefreshToken(claims jwt.MapClaims, config *common.Config) (jwt.MapClaims, bool) {
	iat, ok := claims["iat"].(float64)
	if !ok {
		return nil, false
	}
	exp, ok := claims["exp"].(float64)
	if !ok {
		return nil, false
	}

	now := float64(time.Now().Unix())
	tokenLifetime := exp - iat
	timeUntilExpiry := exp - now

	// Refresh if more than 50% of the token lifetime has passed
	if timeUntilExpiry < tokenLifetime/2 {
		newClaims := make(jwt.MapClaims)
		for k, v := range claims {
			newClaims[k] = v
		}
		// Update timestamps will be done by signAccessToken
		return newClaims, true
	}

	return nil, false
}

// signAccessToken creates a new JWT access token with OAuth 2.1 claims.
func signAccessToken(user *models.InternalUser, clientID, scope string, config *common.Config) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"jti":       uuid.New().String(),
		"sub":       user.UserID,
		"email":     user.Email,
		"name":      user.Name,
		"role":      user.Role,
		"client_id": clientID,
		"scope":     scope,
		"iss":       "vire-server",
		"iat":       now.Unix(),
		"exp":       now.Add(config.Auth.OAuth2.GetAccessTokenExpiry()).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.Auth.JWTSecret))
}

// userContextMiddleware extracts X-Vire-* headers into a UserContext stored
// in the request context. Only creates a UserContext if at least one header
// is present — absent headers mean single-tenant fallback to config defaults.
//
// When X-Vire-User-ID is present and X-Vire-Navexa-Key is absent, the middleware
// looks up the user from storage and resolves their stored navexa_key.
func userContextMiddleware(store interfaces.InternalStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			portfolios := r.Header.Get("X-Vire-Portfolios")
			displayCurrency := r.Header.Get("X-Vire-Display-Currency")
			navexaKey := r.Header.Get("X-Vire-Navexa-Key")
			userID := r.Header.Get("X-Vire-User-ID")
			serviceID := r.Header.Get("X-Vire-Service-ID")

			if portfolios != "" || displayCurrency != "" || navexaKey != "" || userID != "" || serviceID != "" {
				uc := &common.UserContext{}
				if userID != "" {
					uc.UserID = userID
				}

				// Resolve user profile fields from InternalStore (base layer)
				if userID != "" && store != nil {
					if user, err := store.GetUser(r.Context(), userID); err == nil {
						uc.Role = user.Role
						// Load per-user KV entries for preferences
						if kvs, err := store.ListUserKV(r.Context(), userID); err == nil {
							for _, kv := range kvs {
								switch kv.Key {
								case "navexa_key":
									uc.NavexaAPIKey = kv.Value
								case "display_currency":
									uc.DisplayCurrency = kv.Value
								case "portfolios":
									parts := strings.Split(kv.Value, ",")
									for i := range parts {
										parts[i] = strings.TrimSpace(parts[i])
									}
									uc.Portfolios = parts
								}
							}
						}
					}
				}

				// X-Vire-Service-ID resolution (lowest priority — after Bearer and X-Vire-User-ID)
				// If no user identity was resolved, use service identity instead.
				// If user identity exists, service role overrides for authorization.
				if serviceID != "" && store != nil {
					if svcUser, err := store.GetUser(r.Context(), serviceID); err == nil && svcUser.Role == models.RoleService {
						if uc.UserID == "" {
							uc.UserID = svcUser.UserID
						}
						uc.Role = svcUser.Role
					}
				}

				// Header overrides take precedence (backward compat for direct API use)
				if portfolios != "" {
					parts := strings.Split(portfolios, ",")
					for i := range parts {
						parts[i] = strings.TrimSpace(parts[i])
					}
					uc.Portfolios = parts
				}
				if displayCurrency != "" {
					uc.DisplayCurrency = strings.TrimSpace(displayCurrency)
				}
				if navexaKey != "" {
					uc.NavexaAPIKey = navexaKey
				}

				r = r.WithContext(common.WithUserContext(r.Context(), uc))
			}

			next.ServeHTTP(w, r)
		})
	}
}

// applyMiddleware wraps a handler with the middleware stack.
func applyMiddleware(handler http.Handler, logger *common.Logger, config *common.Config, store interfaces.InternalStore) http.Handler {
	// Apply in reverse order (last applied = first executed)
	handler = loggingMiddleware(logger)(handler)
	handler = correlationIDMiddleware(handler)
	handler = userContextMiddleware(store)(handler)
	handler = bearerTokenMiddleware(config, store)(handler)
	handler = corsMiddleware(handler)
	handler = recoveryMiddleware(logger)(handler)
	return handler
}
