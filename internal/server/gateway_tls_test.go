package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/dooservice/gitd/internal/client"
)

// selfSigned writes a certificate for 127.0.0.1 and its key, and answers their files.
func selfSigned(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "gitd test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().String()
}

// TestTheNetworkAddressServesTheAPIOverTLSToAKeyOnly serves the implicit part on the
// network over TLS with its key, and calls it as a platform on another machine would.
func TestTheNetworkAddressServesTheAPIOverTLSToAKeyOnly(t *testing.T) {
	acme := newTenantPart(t, "acme", "acme/web")
	certFile, keyFile := selfSigned(t)
	address := freeAddress(t)
	socketDir, err := os.MkdirTemp("", "gitd-tls-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	gateway := &Gateway{
		Socket: filepath.Join(socketDir, "gitd.sock"), Implicit: acme.tenant,
		Network: &Network{Listen: address, TLSCertFile: certFile, TLSKeyFile: keyFile},
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- gateway.Serve(ctx) }()
	t.Cleanup(func() { cancel(); <-served })
	time.Sleep(300 * time.Millisecond)

	keyed, err := client.Open(client.Options{URL: "https://" + address, Key: "acme-key", CAFile: certFile})
	if err != nil {
		t.Fatal(err)
	}
	if info, err := keyed.Info(ctx); err != nil || info.GetVersion() != "acme" {
		t.Fatalf("over TLS with the key: %v, %v", info, err)
	}
	unkeyed, _ := client.Open(client.Options{URL: "https://" + address, CAFile: certFile})
	if _, err := unkeyed.Info(ctx); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("the API answered on the network without a key: %v", err)
	}
	untrusting, _ := client.Open(client.Options{URL: "https://" + address, Key: "acme-key"})
	if _, err := untrusting.Info(ctx); err == nil {
		t.Error("a client that does not trust the certificate was answered")
	}
	onSocket, _ := client.Open(client.Options{Socket: gateway.Socket})
	if _, err := onSocket.Info(ctx); err != nil {
		t.Errorf("the socket needs no key without tenants: %v", err)
	}
}
