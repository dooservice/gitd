// Package githubapp makes the tokens of a GitHub App: a short JWT signed with the App's
// private key asks GitHub for an installation token limited to one repository and to what
// one operation needs, kept in memory until shortly before it expires.
package githubapp

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"strconv"
	"time"
)

// How long a JWT is good for: GitHub takes at most ten minutes, and its clock may be a
// little behind ours.
const (
	jwtLifetime = 9 * time.Minute
	clockSkew   = time.Minute
)

// KeyLoader reads an App's private key, in PEM. It is called each time a JWT is signed, so
// a key replaced where it is kept is used from then on.
type KeyLoader func() ([]byte, error)

// Signer signs the JWTs an App authenticates to GitHub with.
type Signer struct {
	appID int64
	key   KeyLoader
}

// NewSigner returns the signer of an App, checking that its key can be read now.
func NewSigner(appID int64, key KeyLoader) (*Signer, error) {
	signer := &Signer{appID: appID, key: key}
	if _, err := signer.privateKey(); err != nil {
		return nil, err
	}
	return signer, nil
}

// JWT is a token that says, until it expires, that this App is asking.
func (signer *Signer) JWT(now time.Time) (string, error) {
	key, err := signer.privateKey()
	if err != nil {
		return "", err
	}
	header := encoded(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims := encoded(map[string]any{
		"iat": now.Add(-clockSkew).Unix(),
		"exp": now.Add(jwtLifetime).Unix(),
		"iss": strconv.FormatInt(signer.appID, 10),
	})
	signed := header + "." + claims
	digest := sha256.Sum256([]byte(signed))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signed + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// privateKey reads and parses the App's key, as GitHub gives it (PKCS #1) or in PKCS #8.
func (signer *Signer) privateKey() (*rsa.PrivateKey, error) {
	data, err := signer.key()
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("the App's private key is not in PEM")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("the App's private key is neither PKCS #1 nor PKCS #8")
	}
	key, isRSA := parsed.(*rsa.PrivateKey)
	if !isRSA {
		return nil, errors.New("the App's private key is not an RSA key")
	}
	return key, nil
}

// encoded is a JWT part: JSON, in unpadded base64url.
func encoded(part any) string {
	data, _ := json.Marshal(part)
	return base64.RawURLEncoding.EncodeToString(data)
}
