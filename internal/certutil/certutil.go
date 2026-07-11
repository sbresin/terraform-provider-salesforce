package certutil

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
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
