package mtls

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/luraproject/lura/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCerts holds the paths and data for test certificates.
type testCerts struct {
	caFile     string
	certFile   string
	keyFile    string
	caCertPool *x509.CertPool
	caCert     *x509.Certificate
	caKey      *ecdsa.PrivateKey
}

// generateTestCerts creates a self-signed CA and client certificate in a temp directory.
func generateTestCerts(t *testing.T) *testCerts {
	t.Helper()

	dir := t.TempDir()

	// Generate CA key
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	// CA certificate template
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test CA"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)

	caCert, err := x509.ParseCertificate(caCertDER)
	require.NoError(t, err)

	// Write CA cert PEM
	caFile := filepath.Join(dir, "ca.pem")
	writePEM(t, caFile, "CERTIFICATE", caCertDER)

	// Generate client key
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	// Client certificate template
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{"Test Client"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
		},
	}

	clientCertDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, &clientKey.PublicKey, caKey)
	require.NoError(t, err)

	// Write client cert PEM
	certFile := filepath.Join(dir, "client.pem")
	writePEM(t, certFile, "CERTIFICATE", clientCertDER)

	// Write client key PEM
	keyFile := filepath.Join(dir, "client-key.pem")
	keyDER, err := x509.MarshalECPrivateKey(clientKey)
	require.NoError(t, err)
	writePEM(t, keyFile, "EC PRIVATE KEY", keyDER)

	pool := x509.NewCertPool()
	pool.AddCert(caCert)

	return &testCerts{
		caFile:     caFile,
		certFile:   certFile,
		keyFile:    keyFile,
		caCertPool: pool,
		caCert:     caCert,
		caKey:      caKey,
	}
}

func writePEM(t *testing.T, path, pemType string, data []byte) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, pem.Encode(f, &pem.Block{Type: pemType, Bytes: data}))
}

func TestMTLSConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  MTLSConfig
		wantErr bool
	}{
		{
			name:    "empty CertFile is invalid",
			config:  MTLSConfig{KeyFile: "key.pem"},
			wantErr: true,
		},
		{
			name:    "empty KeyFile is invalid",
			config:  MTLSConfig{CertFile: "cert.pem"},
			wantErr: true,
		},
		{
			name: "TLS 1.0 is rejected",
			config: MTLSConfig{
				CertFile:      "cert.pem",
				KeyFile:       "key.pem",
				MinTLSVersion: tls.VersionTLS10,
			},
			wantErr: true,
		},
		{
			name: "TLS 1.1 is rejected",
			config: MTLSConfig{
				CertFile:      "cert.pem",
				KeyFile:       "key.pem",
				MinTLSVersion: tls.VersionTLS11,
			},
			wantErr: true,
		},
		{
			name: "TLS 1.2 is valid",
			config: MTLSConfig{
				CertFile:      "cert.pem",
				KeyFile:       "key.pem",
				MinTLSVersion: tls.VersionTLS12,
			},
			wantErr: false,
		},
		{
			name: "TLS 1.3 is valid",
			config: MTLSConfig{
				CertFile:      "cert.pem",
				KeyFile:       "key.pem",
				MinTLSVersion: tls.VersionTLS13,
			},
			wantErr: false,
		},
		{
			name: "default MinTLSVersion (zero) is valid",
			config: MTLSConfig{
				CertFile: "cert.pem",
				KeyFile:  "key.pem",
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
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMTLSBlock_Name(t *testing.T) {
	t.Parallel()

	tc := generateTestCerts(t)
	block, err := NewMTLSBlock(MTLSConfig{
		CertFile: tc.certFile,
		KeyFile:  tc.keyFile,
		CAFile:   tc.caFile,
	})
	require.NoError(t, err)
	assert.Equal(t, "mtls", block.Name())
}

func TestMTLSBlock_InvalidCertPath(t *testing.T) {
	t.Parallel()

	_, err := NewMTLSBlock(MTLSConfig{
		CertFile: "/nonexistent/cert.pem",
		KeyFile:  "/nonexistent/key.pem",
	})
	require.Error(t, err)
}

func TestMTLSBlock_MinVersionTLS12(t *testing.T) {
	t.Parallel()

	tc := generateTestCerts(t)
	block, err := NewMTLSBlock(MTLSConfig{
		CertFile: tc.certFile,
		KeyFile:  tc.keyFile,
		CAFile:   tc.caFile,
	})
	require.NoError(t, err)

	// Verify the tls.Config enforces TLS 1.2 minimum
	assert.Equal(t, uint16(tls.VersionTLS12), block.tlsConfig.MinVersion)
}

func TestMTLSBlock_TLSConfigInContext(t *testing.T) {
	t.Parallel()

	tc := generateTestCerts(t)
	block, err := NewMTLSBlock(MTLSConfig{
		CertFile: tc.certFile,
		KeyFile:  tc.keyFile,
		CAFile:   tc.caFile,
	})
	require.NoError(t, err)

	var capturedCtx context.Context
	nextProxy := func(ctx context.Context, _ *proxy.Request) (*proxy.Response, error) {
		capturedCtx = ctx
		return &proxy.Response{
			IsComplete: true,
			Metadata: proxy.Metadata{
				StatusCode: 200,
				Headers:    http.Header{},
			},
		}, nil
	}

	mw := block.Middleware()
	p := mw(nextProxy)

	req := &proxy.Request{
		Method:  http.MethodGet,
		Headers: map[string][]string{},
	}

	_, err = p(context.Background(), req)
	require.NoError(t, err)

	// Verify tls.Config was stored in context
	tlsCfg := GetTLSConfig(capturedCtx)
	require.NotNil(t, tlsCfg)
	assert.Equal(t, uint16(tls.VersionTLS12), tlsCfg.MinVersion)
	assert.NotNil(t, tlsCfg.GetClientCertificate, "GetClientCertificate callback must be set")
}

func TestMTLSBlock_GetClientCertificateCallback(t *testing.T) {
	t.Parallel()

	tc := generateTestCerts(t)
	block, err := NewMTLSBlock(MTLSConfig{
		CertFile: tc.certFile,
		KeyFile:  tc.keyFile,
		CAFile:   tc.caFile,
	})
	require.NoError(t, err)

	// Verify GetClientCertificate loads the cert each time (for rotation support)
	require.NotNil(t, block.tlsConfig.GetClientCertificate)

	cert1, err := block.tlsConfig.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.NoError(t, err)
	require.NotNil(t, cert1)
	assert.NotEmpty(t, cert1.Certificate, "should have loaded certificate")

	// Call again -- should still work (re-loads from disk)
	cert2, err := block.tlsConfig.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.NoError(t, err)
	require.NotNil(t, cert2)
}

func TestMTLSBlock_CABundle(t *testing.T) {
	t.Parallel()

	tc := generateTestCerts(t)
	block, err := NewMTLSBlock(MTLSConfig{
		CertFile: tc.certFile,
		KeyFile:  tc.keyFile,
		CAFile:   tc.caFile,
	})
	require.NoError(t, err)

	assert.NotNil(t, block.tlsConfig.RootCAs, "RootCAs should be set when CAFile is provided")
}

func TestMTLSBlock_PresentsCertToUpstream(t *testing.T) {
	t.Parallel()

	tc := generateTestCerts(t)

	// Create a TLS server that requires client certificates
	clientCertReceived := false

	serverTLSCert := generateServerCert(t, tc)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			clientCertReceived = true
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))

	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverTLSCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    tc.caCertPool,
		MinVersion:   tls.VersionTLS12,
	}
	server.StartTLS()
	defer server.Close()

	block, err := NewMTLSBlock(MTLSConfig{
		CertFile:           tc.certFile,
		KeyFile:            tc.keyFile,
		CAFile:             tc.caFile,
		InsecureSkipVerify: true, // Test server has self-signed cert
	})
	require.NoError(t, err)

	// Use the TLS config from block to make an actual HTTPS call
	transport := &http.Transport{
		TLSClientConfig: block.tlsConfig,
	}
	client := &http.Client{Transport: transport}

	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, clientCertReceived, "server should have received client certificate")
}

func TestMTLSBlock_ImplementsBlockInterface(t *testing.T) {
	t.Parallel()

	tc := generateTestCerts(t)
	block, err := NewMTLSBlock(MTLSConfig{
		CertFile: tc.certFile,
		KeyFile:  tc.keyFile,
		CAFile:   tc.caFile,
	})
	require.NoError(t, err)

	var _ interface {
		Name() string
		Middleware() proxy.Middleware
	} = block
}

// generateServerCert creates a server certificate signed by the test CA.
func generateServerCert(t *testing.T, tc *testCerts) tls.Certificate {
	t.Helper()

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject: pkix.Name{
			Organization: []string{"Test Server"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}

	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, tc.caCert, &serverKey.PublicKey, tc.caKey)
	require.NoError(t, err)

	return tls.Certificate{
		Certificate: [][]byte{serverCertDER},
		PrivateKey:  serverKey,
	}
}
