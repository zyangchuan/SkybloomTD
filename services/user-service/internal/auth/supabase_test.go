package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestParseAuthorizationHeaderAcceptsES256P256JWKS(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate private key: %v", err)
	}

	const keyID = "test-ec-p256-key"
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(jwksDocument{
			Keys: []jwkKey{
				{
					KID: keyID,
					KTY: "EC",
					ALG: "ES256",
					CRV: "P-256",
					X:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.X.Bytes()),
					Y:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.Y.Bytes()),
				},
			},
		}); err != nil {
			t.Fatalf("encode jwks: %v", err)
		}
	}))
	defer jwksServer.Close()

	token := jwt.NewWithClaims(jwt.SigningMethodES256, SupabaseClaims{
		Email: "student@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "7b472c87-6aeb-41f4-983f-072b0ee24f69",
			Audience:  jwt.ClaimStrings{"authenticated"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	token.Header["kid"] = keyID

	rawToken, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	parser := Parser{
		JWTAudience: "authenticated",
		JWKSURL:     jwksServer.URL,
	}
	claims, err := parser.ParseAuthorizationHeader("Bearer " + rawToken)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.Subject != "7b472c87-6aeb-41f4-983f-072b0ee24f69" {
		t.Fatalf("subject mismatch: got %q", claims.Subject)
	}
	if claims.Email != "student@example.com" {
		t.Fatalf("email mismatch: got %q", claims.Email)
	}
}
