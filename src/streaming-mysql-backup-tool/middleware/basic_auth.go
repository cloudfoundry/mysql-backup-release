package middleware

import (
	"crypto/subtle"
	"net/http"
)

func BasicAuth(next http.Handler, requiredUsername, requiredPassword string) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		username, password, ok := req.BasicAuth()
		if ok &&
			secureCompare(username, requiredUsername) &&
			secureCompare(password, requiredPassword) {
			next.ServeHTTP(rw, req)
		} else {
			rw.Header().Set("WWW-Authenticate", "Basic realm=\"Authorization Required\"")
			http.Error(rw, "Not Authorized", http.StatusUnauthorized)
		}
	})
}

func secureCompare(v1, v2 string) bool {
	return subtle.ConstantTimeCompare([]byte(v1), []byte(v2)) == 1
}
