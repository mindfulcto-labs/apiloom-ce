package jwt

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cto-venkat/apiloom-ce/internal/block/auth"
	"github.com/cto-venkat/apiloom-ce/internal/domain"
	"github.com/cto-venkat/apiloom-ce/internal/gateway"
	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/luraproject/lura/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- test helpers ----------

// testKeys holds generated keys for test signing and verification.
type testKeys struct {
	rsaPriv  *rsa.PrivateKey
	rsaPub   *rsa.PublicKey
	ecPriv   *ecdsa.PrivateKey
	ecPub    *ecdsa.PublicKey
	hmacKey  []byte
	jwksData []byte
	server   *httptest.Server
}

func newTestKeys(t *testing.T) *testKeys {
	t.Helper()

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	hmacSecret := []byte("test-hmac-secret-key-32-bytes!!")

	// Build JWKS
	set := jwk.NewSet()

	rsaJWK, err := jwk.FromRaw(rsaKey.PublicKey)
	require.NoError(t, err)
	require.NoError(t, rsaJWK.Set(jwk.KeyIDKey, "rsa-key-1"))
	require.NoError(t, rsaJWK.Set(jwk.AlgorithmKey, jwa.RS256))
	require.NoError(t, rsaJWK.Set(jwk.KeyUsageKey, "sig"))
	require.NoError(t, set.AddKey(rsaJWK))

	ecJWK, err := jwk.FromRaw(ecKey.PublicKey)
	require.NoError(t, err)
	require.NoError(t, ecJWK.Set(jwk.KeyIDKey, "ec-key-1"))
	require.NoError(t, ecJWK.Set(jwk.AlgorithmKey, jwa.ES256))
	require.NoError(t, ecJWK.Set(jwk.KeyUsageKey, "sig"))
	require.NoError(t, set.AddKey(ecJWK))

	hmacJWK, err := jwk.FromRaw(hmacSecret)
	require.NoError(t, err)
	require.NoError(t, hmacJWK.Set(jwk.KeyIDKey, "hmac-key-1"))
	require.NoError(t, hmacJWK.Set(jwk.AlgorithmKey, jwa.HS256))
	require.NoError(t, hmacJWK.Set(jwk.KeyUsageKey, "sig"))
	require.NoError(t, set.AddKey(hmacJWK))

	jwksBytes, err := json.Marshal(set)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(jwksBytes)
	}))
	t.Cleanup(srv.Close)

	return &testKeys{
		rsaPriv:  rsaKey,
		rsaPub:   &rsaKey.PublicKey,
		ecPriv:   ecKey,
		ecPub:    &ecKey.PublicKey,
		hmacKey:  hmacSecret,
		jwksData: jwksBytes,
		server:   srv,
	}
}

func (tk *testKeys) validConfig() JWTConfig {
	return JWTConfig{
		JWKSEndpoint:     tk.server.URL,
		AllowedIssuers:   []string{"https://auth.example.com"},
		AllowedAudiences: []string{"my-api"},
		ClaimMap: map[string]string{
			"sub":   "X-User-Sub",
			"email": "X-User-Email",
		},
	}
}

func buildToken(t *testing.T, key interface{}, alg jwa.SignatureAlgorithm, kid string, claims map[string]interface{}) string {
	t.Helper()
	builder := jwt.New()
	for k, v := range claims {
		require.NoError(t, builder.Set(k, v))
	}

	hdrs := jwa.SignatureAlgorithm(alg)
	_ = hdrs

	opts := []jwt.SignOption{jwt.WithKey(alg, key)}
	if kid != "" {
		jwkKey, err := jwk.FromRaw(key)
		require.NoError(t, err)
		require.NoError(t, jwkKey.Set(jwk.KeyIDKey, kid))
		require.NoError(t, jwkKey.Set(jwk.AlgorithmKey, alg))
		opts = []jwt.SignOption{jwt.WithKey(alg, jwkKey)}
	}

	signed, err := jwt.Sign(builder, opts...)
	require.NoError(t, err)
	return string(signed)
}

func validClaims() map[string]interface{} {
	now := time.Now()
	return map[string]interface{}{
		"iss":   "https://auth.example.com",
		"aud":   []string{"my-api"},
		"sub":   "user-123",
		"email": "user@example.com",
		"exp":   now.Add(1 * time.Hour).Unix(),
		"iat":   now.Unix(),
		"nbf":   now.Add(-1 * time.Minute).Unix(),
	}
}

func makeRequest(t *testing.T, authHeader string) *proxy.Request {
	t.Helper()
	req := &proxy.Request{
		Headers: map[string][]string{},
		Params:  map[string]string{},
	}
	if authHeader != "" {
		req.Headers["Authorization"] = []string{authHeader}
	}
	return req
}

func assertProblemError(t *testing.T, err error, status int, errType string) {
	t.Helper()
	var pe *domain.ProblemError
	require.True(t, errors.As(err, &pe), "expected ProblemError, got: %T: %v", err, err)
	assert.Equal(t, status, pe.Status)
	assert.Contains(t, pe.Type, errType)
}

// ---------- Config validation tests ----------

func TestJWTConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  JWTConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty JWKS URL",
			config:  JWTConfig{AllowedIssuers: []string{"iss"}, AllowedAudiences: []string{"aud"}},
			wantErr: true,
			errMsg:  "jwks_endpoint",
		},
		{
			name:    "empty AllowedIssuers",
			config:  JWTConfig{JWKSEndpoint: "https://example.com/.well-known/jwks.json", AllowedAudiences: []string{"aud"}},
			wantErr: true,
			errMsg:  "allowed_issuers",
		},
		{
			name:    "empty AllowedAudiences",
			config:  JWTConfig{JWKSEndpoint: "https://example.com/.well-known/jwks.json", AllowedIssuers: []string{"iss"}},
			wantErr: true,
			errMsg:  "allowed_audiences",
		},
		{
			name: "valid config",
			config: JWTConfig{
				JWKSEndpoint:     "https://example.com/.well-known/jwks.json",
				AllowedIssuers:   []string{"iss"},
				AllowedAudiences: []string{"aud"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ---------- Token extraction tests ----------

func TestJWTBlock_TokenExtraction(t *testing.T) {
	t.Parallel()
	tk := newTestKeys(t)
	ctx := context.Background()

	block, err := NewJWTBlock(ctx, tk.validConfig())
	require.NoError(t, err)

	token := buildToken(t, tk.rsaPriv, jwa.RS256, "rsa-key-1", validClaims())
	mw := block.Middleware()
	nextProxy := func(_ context.Context, req *proxy.Request) (*proxy.Response, error) {
		return &proxy.Response{IsComplete: true, Metadata: proxy.Metadata{Headers: req.Headers}}, nil
	}
	p := mw(nextProxy)

	tests := []struct {
		name       string
		authHeader string
		wantErr    bool
		errType    string
	}{
		{
			name:       "valid Bearer token extracts correctly",
			authHeader: "Bearer " + token,
			wantErr:    false,
		},
		{
			name:       "missing Authorization header",
			authHeader: "",
			wantErr:    true,
			errType:    "missing_token",
		},
		{
			name:       "Basic auth instead of Bearer",
			authHeader: "Basic dXNlcjpwYXNz",
			wantErr:    true,
			errType:    "missing_token",
		},
		{
			name:       "Bearer with empty token",
			authHeader: "Bearer ",
			wantErr:    true,
			errType:    "missing_token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := makeRequest(t, tt.authHeader)
			resp, err := p(ctx, req)
			if tt.wantErr {
				assertProblemError(t, err, http.StatusUnauthorized, tt.errType)
				assert.Nil(t, resp)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, resp)
			}
		})
	}
}

// ---------- Signature verification tests ----------

func TestJWTBlock_SignatureVerification(t *testing.T) {
	t.Parallel()
	tk := newTestKeys(t)
	ctx := context.Background()

	cfg := tk.validConfig()
	cfg.AllowedAlgorithms = []string{"RS256", "ES256", "HS256"}

	block, err := NewJWTBlock(ctx, cfg)
	require.NoError(t, err)

	mw := block.Middleware()
	nextProxy := func(_ context.Context, req *proxy.Request) (*proxy.Response, error) {
		return &proxy.Response{IsComplete: true}, nil
	}
	p := mw(nextProxy)

	claims := validClaims()

	t.Run("RS256 valid signature passes", func(t *testing.T) {
		t.Parallel()
		token := buildToken(t, tk.rsaPriv, jwa.RS256, "rsa-key-1", claims)
		req := makeRequest(t, "Bearer "+token)
		resp, err := p(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("ES256 valid signature passes", func(t *testing.T) {
		t.Parallel()
		token := buildToken(t, tk.ecPriv, jwa.ES256, "ec-key-1", claims)
		req := makeRequest(t, "Bearer "+token)
		resp, err := p(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("HS256 valid key passes", func(t *testing.T) {
		t.Parallel()
		token := buildToken(t, tk.hmacKey, jwa.HS256, "hmac-key-1", claims)
		req := makeRequest(t, "Bearer "+token)
		resp, err := p(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("alg:none is rejected", func(t *testing.T) {
		t.Parallel()
		// Manually craft an alg:none token
		noneToken := craftAlgNoneToken(t, claims)
		req := makeRequest(t, "Bearer "+noneToken)
		_, err := p(ctx, req)
		assertProblemError(t, err, http.StatusUnauthorized, "invalid_token")
	})

	t.Run("invalid signature rejected", func(t *testing.T) {
		t.Parallel()
		// Sign with a different RSA key
		otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		token := buildToken(t, otherKey, jwa.RS256, "rsa-key-1", claims)
		req := makeRequest(t, "Bearer "+token)
		_, err = p(ctx, req)
		assertProblemError(t, err, http.StatusUnauthorized, "invalid_token")
	})
}

// craftAlgNoneToken creates a token with alg:none to test rejection.
func craftAlgNoneToken(t *testing.T, claims map[string]interface{}) string {
	t.Helper()
	// Build the header and payload manually since jwt.Sign won't sign with none
	headerJSON := `{"alg":"none","typ":"JWT"}`
	claimsJSON, err := json.Marshal(claims)
	require.NoError(t, err)

	header := base64URLEncode([]byte(headerJSON))
	payload := base64URLEncode(claimsJSON)
	// alg:none token has empty signature
	return header + "." + payload + "."
}

func base64URLEncode(data []byte) string {
	encoded := make([]byte, base64URLEncodedLen(len(data)))
	base64URLEncodeBytes(encoded, data)
	return string(encoded)
}

func base64URLEncodedLen(n int) int {
	return (n*8 + 5) / 6
}

func base64URLEncodeBytes(dst, src []byte) {
	const encodeURL = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	di, si := 0, 0
	n := (len(src) / 3) * 3
	for si < n {
		val := uint(src[si])<<16 | uint(src[si+1])<<8 | uint(src[si+2])
		dst[di+0] = encodeURL[val>>18&0x3F]
		dst[di+1] = encodeURL[val>>12&0x3F]
		dst[di+2] = encodeURL[val>>6&0x3F]
		dst[di+3] = encodeURL[val>>0&0x3F]
		si += 3
		di += 4
	}
	remain := len(src) - si
	if remain == 0 {
		return
	}
	val := uint(src[si]) << 16
	if remain == 2 {
		val |= uint(src[si+1]) << 8
	}
	dst[di+0] = encodeURL[val>>18&0x3F]
	dst[di+1] = encodeURL[val>>12&0x3F]
	if remain == 2 {
		dst[di+2] = encodeURL[val>>6&0x3F]
	}
}

// ---------- Claim validation tests ----------

func TestJWTBlock_ClaimValidation(t *testing.T) {
	t.Parallel()
	tk := newTestKeys(t)
	ctx := context.Background()

	block, err := NewJWTBlock(ctx, tk.validConfig())
	require.NoError(t, err)

	mw := block.Middleware()
	nextProxy := func(_ context.Context, req *proxy.Request) (*proxy.Response, error) {
		return &proxy.Response{IsComplete: true}, nil
	}
	p := mw(nextProxy)

	t.Run("expired token rejected", func(t *testing.T) {
		t.Parallel()
		claims := validClaims()
		claims["exp"] = time.Now().Add(-1 * time.Hour).Unix()
		token := buildToken(t, tk.rsaPriv, jwa.RS256, "rsa-key-1", claims)
		req := makeRequest(t, "Bearer "+token)
		_, err := p(ctx, req)
		assertProblemError(t, err, http.StatusUnauthorized, "invalid_token")
	})

	t.Run("nbf in future rejected", func(t *testing.T) {
		t.Parallel()
		claims := validClaims()
		claims["nbf"] = time.Now().Add(1 * time.Hour).Unix()
		token := buildToken(t, tk.rsaPriv, jwa.RS256, "rsa-key-1", claims)
		req := makeRequest(t, "Bearer "+token)
		_, err := p(ctx, req)
		assertProblemError(t, err, http.StatusUnauthorized, "invalid_token")
	})

	t.Run("wrong issuer rejected", func(t *testing.T) {
		t.Parallel()
		claims := validClaims()
		claims["iss"] = "https://evil.example.com"
		token := buildToken(t, tk.rsaPriv, jwa.RS256, "rsa-key-1", claims)
		req := makeRequest(t, "Bearer "+token)
		_, err := p(ctx, req)
		assertProblemError(t, err, http.StatusUnauthorized, "invalid_token")
	})

	t.Run("wrong audience rejected", func(t *testing.T) {
		t.Parallel()
		claims := validClaims()
		claims["aud"] = []string{"wrong-api"}
		token := buildToken(t, tk.rsaPriv, jwa.RS256, "rsa-key-1", claims)
		req := makeRequest(t, "Bearer "+token)
		_, err := p(ctx, req)
		assertProblemError(t, err, http.StatusUnauthorized, "invalid_token")
	})

	t.Run("valid iss and aud passes", func(t *testing.T) {
		t.Parallel()
		token := buildToken(t, tk.rsaPriv, jwa.RS256, "rsa-key-1", validClaims())
		req := makeRequest(t, "Bearer "+token)
		resp, err := p(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})
}

// ---------- Claim extraction tests ----------

func TestJWTBlock_ClaimExtraction(t *testing.T) {
	t.Parallel()
	tk := newTestKeys(t)
	ctx := context.Background()

	cfg := tk.validConfig()
	cfg.ClaimMap = map[string]string{
		"sub":   "X-User-Sub",
		"email": "X-User-Email",
	}

	block, err := NewJWTBlock(ctx, cfg)
	require.NoError(t, err)

	mw := block.Middleware()

	t.Run("configured claims mapped to headers", func(t *testing.T) {
		t.Parallel()
		var capturedReq *proxy.Request
		nextProxy := func(_ context.Context, req *proxy.Request) (*proxy.Response, error) {
			capturedReq = req
			return &proxy.Response{IsComplete: true, Metadata: proxy.Metadata{Headers: req.Headers}}, nil
		}
		p := mw(nextProxy)

		token := buildToken(t, tk.rsaPriv, jwa.RS256, "rsa-key-1", validClaims())
		req := makeRequest(t, "Bearer "+token)
		_, err := p(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, capturedReq)

		assert.Equal(t, []string{"user-123"}, capturedReq.Headers["X-User-Sub"])
		assert.Equal(t, []string{"user@example.com"}, capturedReq.Headers["X-User-Email"])
	})

	t.Run("non-allowlisted claims NOT forwarded", func(t *testing.T) {
		t.Parallel()
		var capturedReq *proxy.Request
		nextProxy := func(_ context.Context, req *proxy.Request) (*proxy.Response, error) {
			capturedReq = req
			return &proxy.Response{IsComplete: true, Metadata: proxy.Metadata{Headers: req.Headers}}, nil
		}
		p := mw(nextProxy)

		claims := validClaims()
		claims["custom_role"] = "admin"
		token := buildToken(t, tk.rsaPriv, jwa.RS256, "rsa-key-1", claims)
		req := makeRequest(t, "Bearer "+token)
		_, err := p(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, capturedReq)

		// custom_role is NOT in ClaimMap, so should not appear as header
		for k := range capturedReq.Headers {
			assert.NotContains(t, k, "custom_role")
			assert.NotContains(t, k, "Custom-Role")
			assert.NotContains(t, k, "X-Custom-Role")
		}
	})

	t.Run("missing optional claim does not add empty header", func(t *testing.T) {
		t.Parallel()
		var capturedReq *proxy.Request
		nextProxy := func(_ context.Context, req *proxy.Request) (*proxy.Response, error) {
			capturedReq = req
			return &proxy.Response{IsComplete: true, Metadata: proxy.Metadata{Headers: req.Headers}}, nil
		}
		p := mw(nextProxy)

		claims := validClaims()
		delete(claims, "email") // remove email claim
		token := buildToken(t, tk.rsaPriv, jwa.RS256, "rsa-key-1", claims)
		req := makeRequest(t, "Bearer "+token)
		_, err := p(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, capturedReq)

		// sub should be present
		assert.Equal(t, []string{"user-123"}, capturedReq.Headers["X-User-Sub"])
		// email should NOT be present (not even as empty)
		_, hasEmail := capturedReq.Headers["X-User-Email"]
		assert.False(t, hasEmail, "missing claim should not produce empty header")
	})
}

// ---------- Consumer context tests ----------

func TestJWTBlock_ConsumerContext(t *testing.T) {
	t.Parallel()
	tk := newTestKeys(t)
	ctx := context.Background()

	block, err := NewJWTBlock(ctx, tk.validConfig())
	require.NoError(t, err)

	mw := block.Middleware()

	t.Run("successful validation sets consumer in context", func(t *testing.T) {
		t.Parallel()
		var capturedCtx context.Context
		nextProxy := func(ctx context.Context, _ *proxy.Request) (*proxy.Response, error) {
			capturedCtx = ctx
			return &proxy.Response{IsComplete: true}, nil
		}
		p := mw(nextProxy)

		token := buildToken(t, tk.rsaPriv, jwa.RS256, "rsa-key-1", validClaims())
		req := makeRequest(t, "Bearer "+token)
		_, err := p(ctx, req)
		require.NoError(t, err)

		consumer := auth.GetConsumer(capturedCtx)
		require.NotNil(t, consumer, "consumer should be set in context after successful validation")
		assert.Equal(t, "user-123", consumer.Name)
	})
}

// ---------- JWKS cache tests ----------

func TestJWTBlock_JWKSCache(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("fetches JWKS at startup", func(t *testing.T) {
		t.Parallel()
		fetchCount := 0
		rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		set := jwk.NewSet()
		pubJWK, err := jwk.FromRaw(rsaKey.PublicKey)
		require.NoError(t, err)
		require.NoError(t, pubJWK.Set(jwk.KeyIDKey, "test-key"))
		require.NoError(t, pubJWK.Set(jwk.AlgorithmKey, jwa.RS256))
		require.NoError(t, set.AddKey(pubJWK))
		jwksBytes, err := json.Marshal(set)
		require.NoError(t, err)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fetchCount++
			w.Header().Set("Content-Type", "application/json")
			w.Write(jwksBytes)
		}))
		t.Cleanup(srv.Close)

		cfg := JWTConfig{
			JWKSEndpoint:     srv.URL,
			AllowedIssuers:   []string{"https://auth.example.com"},
			AllowedAudiences: []string{"my-api"},
		}
		_, err = NewJWTBlock(ctx, cfg)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, fetchCount, 1, "should fetch JWKS at startup")
	})

	t.Run("unknown kid triggers refresh", func(t *testing.T) {
		t.Parallel()
		rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		// Start with empty keyset, then add the key on second fetch
		set1 := jwk.NewSet()
		set2 := jwk.NewSet()
		pubJWK, err := jwk.FromRaw(rsaKey.PublicKey)
		require.NoError(t, err)
		require.NoError(t, pubJWK.Set(jwk.KeyIDKey, "new-key"))
		require.NoError(t, pubJWK.Set(jwk.AlgorithmKey, jwa.RS256))
		require.NoError(t, set2.AddKey(pubJWK))

		jwks1, err := json.Marshal(set1)
		require.NoError(t, err)
		jwks2, err := json.Marshal(set2)
		require.NoError(t, err)

		reqCount := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			reqCount++
			w.Header().Set("Content-Type", "application/json")
			if reqCount <= 1 {
				w.Write(jwks1) // empty keyset first
			} else {
				w.Write(jwks2) // key appears on refresh
			}
		}))
		t.Cleanup(srv.Close)

		cfg := JWTConfig{
			JWKSEndpoint:     srv.URL,
			AllowedIssuers:   []string{"https://auth.example.com"},
			AllowedAudiences: []string{"my-api"},
		}
		block, err := NewJWTBlock(ctx, cfg)
		require.NoError(t, err)

		mw := block.Middleware()
		nextProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
			return &proxy.Response{IsComplete: true}, nil
		}
		p := mw(nextProxy)

		// Sign with the key that was not in the initial keyset
		claims := validClaims()
		token := buildToken(t, rsaKey, jwa.RS256, "new-key", claims)
		req := makeRequest(t, "Bearer "+token)
		resp, err := p(ctx, req)
		require.NoError(t, err, "should succeed after cache refresh finds the key")
		assert.NotNil(t, resp)
		assert.GreaterOrEqual(t, reqCount, 2, "should have fetched JWKS at least twice (initial + refresh)")
	})
}

// ---------- Block interface tests ----------

func TestJWTBlock_BlockInterface(t *testing.T) {
	t.Parallel()
	tk := newTestKeys(t)
	ctx := context.Background()

	block, err := NewJWTBlock(ctx, tk.validConfig())
	require.NoError(t, err)

	t.Run("implements gateway.Block", func(t *testing.T) {
		t.Parallel()
		var _ gateway.Block = block
	})

	t.Run("Name returns auth-jwt", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "auth-jwt", block.Name())
	})

	t.Run("Middleware returns valid proxy.Middleware", func(t *testing.T) {
		t.Parallel()
		mw := block.Middleware()
		assert.NotNil(t, mw)
	})
}

// suppress unused import warnings
var (
	_ = uuid.New
	_ = fmt.Sprintf
)
