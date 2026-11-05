package certutil

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
)

func NormalizePEMFromSalesforce(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("certificate content is empty")
	}

	if strings.Contains(trimmed, "-----BEGIN CERTIFICATE-----") {
		return ensureValidPEM(trimmed)
	}

	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return "", fmt.Errorf("certificate content is neither PEM nor base64-encoded PEM: %w", err)
	}

	return ensureValidPEM(string(decoded))
}

func LeafSHA256(certPEM string) (string, error) {
	cert, err := parseLeaf(certPEM)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(cert.Raw)
	return strings.ToUpper(hex.EncodeToString(sum[:])), nil
}

func ensureValidPEM(certPEM string) (string, error) {
	if _, err := parseLeaf(certPEM); err != nil {
		return "", err
	}

	if strings.HasSuffix(certPEM, "\n") {
		return certPEM, nil
	}
	return certPEM + "\n", nil
}

func parseLeaf(certPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("certificate PEM does not contain a CERTIFICATE block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %w", err)
	}

	return cert, nil
}

// JWK returns the certificate's RSA public key as a JSON Web Key for RS256
// signature verification, identified by kid.
func JWK(certPEM, kid string) (string, error) {
	cert, err := parseLeaf(certPEM)
	if err != nil {
		return "", err
	}
	key, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("certificate public key is %T, not RSA", cert.PublicKey)
	}

	jwk, err := json.Marshal(struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		Use string `json:"use"`
		Alg string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
	}{
		Kty: "RSA",
		Kid: kid,
		Use: "sig",
		Alg: "RS256",
		N:   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	})
	return string(jwk), err
}
