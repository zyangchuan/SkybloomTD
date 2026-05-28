package auth

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"skybloom/user-service/internal/config"
)

const (
	userIDKey = "supabase_user_id"
	claimsKey = "supabase_claims"
)

type SupabaseClaims struct {
	Email        string         `json:"email,omitempty"`
	UserMetadata map[string]any `json:"user_metadata,omitempty"`
	AppMetadata  map[string]any `json:"app_metadata,omitempty"`
	jwt.RegisteredClaims
}

func Middleware(cfg config.Config) gin.HandlerFunc {
	parser := Parser{
		JWTSecret:          cfg.SupabaseJWTSecret,
		JWTAudience:        cfg.SupabaseJWTAudience,
		JWKSURL:            cfg.SupabaseJWKSURL,
		AuthCookieName:     cfg.AuthCookieName,
		AllowUnverifiedJWT: cfg.AllowUnverifiedJWT,
	}

	return func(c *gin.Context) {
		claims, err := parser.ParseRequest(c.Request)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		userID, err := uuid.Parse(claims.Subject)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "jwt subject must be a UUID"})
			c.Abort()
			return
		}

		c.Set(userIDKey, userID)
		c.Set(claimsKey, claims)
		c.Next()
	}
}

func UserID(c *gin.Context) (uuid.UUID, bool) {
	value, exists := c.Get(userIDKey)
	if !exists {
		return uuid.Nil, false
	}
	userID, ok := value.(uuid.UUID)
	return userID, ok
}

func Claims(c *gin.Context) (*SupabaseClaims, bool) {
	value, exists := c.Get(claimsKey)
	if !exists {
		return nil, false
	}
	claims, ok := value.(*SupabaseClaims)
	return claims, ok
}

type Parser struct {
	JWTSecret          string
	JWTAudience        string
	JWKSURL            string
	AuthCookieName     string
	AllowUnverifiedJWT bool

	mu          sync.Mutex
	cachedJWKS  map[string]any
	jwksExpires time.Time
}

func (p *Parser) ParseRequest(request *http.Request) (*SupabaseClaims, error) {
	if request == nil {
		return nil, errors.New("missing request")
	}
	rawToken, ok, err := p.tokenFromCookie(request)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("missing access token cookie")
	}
	return p.ParseToken(rawToken)
}

func (p *Parser) ParseToken(rawToken string) (*SupabaseClaims, error) {
	claims := &SupabaseClaims{}
	if p.AllowUnverifiedJWT {
		parser := jwt.NewParser()
		_, _, err := parser.ParseUnverified(rawToken, claims)
		if err != nil {
			return nil, errors.New("invalid jwt")
		}
		if claims.Subject == "" {
			return nil, errors.New("jwt subject is required")
		}
		return claims, nil
	}

	if p.JWTSecret == "" {
		if p.JWKSURL == "" {
			return nil, errors.New("SUPABASE_JWT_SECRET or SUPABASE_JWKS_URL is required")
		}
	}

	parserOptions := []jwt.ParserOption{
		jwt.WithValidMethods([]string{"HS256", "RS256", "ES256", "EdDSA"}),
	}
	if p.JWTAudience != "" {
		parserOptions = append(parserOptions, jwt.WithAudience(p.JWTAudience))
	}

	token, err := jwt.ParseWithClaims(
		rawToken,
		claims,
		func(token *jwt.Token) (any, error) {
			switch token.Method.Alg() {
			case "HS256":
				if p.JWTSecret == "" {
					return nil, errors.New("SUPABASE_JWT_SECRET is required for HS256 tokens")
				}
				return []byte(p.JWTSecret), nil
			case "RS256", "ES256", "EdDSA":
				return p.keyFromJWKS(token)
			default:
				return nil, fmt.Errorf("unsupported jwt algorithm: %s", token.Method.Alg())
			}
		},
		parserOptions...,
	)
	if err != nil {
		return nil, fmt.Errorf("invalid jwt: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("invalid jwt")
	}
	if claims.Subject == "" {
		return nil, errors.New("jwt subject is required")
	}
	return claims, nil
}

func (p *Parser) tokenFromCookie(request *http.Request) (string, bool, error) {
	cookieName := strings.TrimSpace(p.AuthCookieName)
	if cookieName == "" {
		cookieName = "skybloom_access_token"
	}
	cookie, err := request.Cookie(cookieName)
	if err == http.ErrNoCookie {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	token := strings.TrimSpace(cookie.Value)
	if token == "" {
		return "", false, nil
	}
	return token, true, nil
}

func (p *Parser) keyFromJWKS(token *jwt.Token) (any, error) {
	if p.JWKSURL == "" {
		return nil, errors.New("SUPABASE_JWKS_URL is required for asymmetric tokens")
	}

	kid, _ := token.Header["kid"].(string)
	if kid == "" {
		return nil, errors.New("jwt kid header is required for asymmetric tokens")
	}

	key, ok, err := p.cachedKey(kid)
	if err != nil {
		return nil, err
	}
	if ok {
		return key, nil
	}

	if err := p.refreshJWKS(); err != nil {
		return nil, err
	}
	key, ok, err = p.cachedKey(kid)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("jwt signing key not found in Supabase JWKS")
	}
	return key, nil
}

func (p *Parser) cachedKey(kid string) (any, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cachedJWKS == nil || time.Now().After(p.jwksExpires) {
		return nil, false, nil
	}
	key, ok := p.cachedJWKS[kid]
	return key, ok, nil
}

func (p *Parser) refreshJWKS() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cachedJWKS != nil && time.Now().Before(p.jwksExpires) {
		return nil
	}

	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(p.JWKSURL)
	if err != nil {
		return fmt.Errorf("fetch Supabase JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("fetch Supabase JWKS: status %d", resp.StatusCode)
	}

	var jwks jwksDocument
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("decode Supabase JWKS: %w", err)
	}

	keys := make(map[string]any, len(jwks.Keys))
	for _, jwk := range jwks.Keys {
		key, err := jwk.publicKey()
		if err != nil {
			continue
		}
		if jwk.KID != "" {
			keys[jwk.KID] = key
		}
	}
	if len(keys) == 0 {
		return errors.New("Supabase JWKS did not contain any supported signing keys")
	}

	p.cachedJWKS = keys
	p.jwksExpires = time.Now().Add(10 * time.Minute)
	return nil
}

type jwksDocument struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	KID string `json:"kid"`
	KTY string `json:"kty"`
	ALG string `json:"alg"`
	CRV string `json:"crv"`
	N   string `json:"n"`
	E   string `json:"e"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func (k jwkKey) publicKey() (any, error) {
	switch k.KTY {
	case "RSA":
		return k.rsaPublicKey()
	case "EC":
		return k.ecdsaPublicKey()
	case "OKP":
		return k.ed25519PublicKey()
	default:
		return nil, fmt.Errorf("unsupported jwk key type: %s", k.KTY)
	}
}

func (k jwkKey) rsaPublicKey() (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, err
	}

	exponent := 0
	for _, b := range eBytes {
		exponent = exponent<<8 + int(b)
	}
	if exponent == 0 {
		return nil, errors.New("invalid RSA exponent")
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: exponent,
	}, nil
}

func (k jwkKey) ecdsaPublicKey() (*ecdsa.PublicKey, error) {
	if k.CRV != "P-256" {
		return nil, fmt.Errorf("unsupported EC curve: %s", k.CRV)
	}

	xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, err
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(k.Y)
	if err != nil {
		return nil, err
	}

	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}, nil
}

func (k jwkKey) ed25519PublicKey() (ed25519.PublicKey, error) {
	if k.CRV != "Ed25519" {
		return nil, fmt.Errorf("unsupported OKP curve: %s", k.CRV)
	}
	xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, err
	}
	if len(xBytes) != ed25519.PublicKeySize {
		return nil, errors.New("invalid Ed25519 public key size")
	}
	return ed25519.PublicKey(xBytes), nil
}
