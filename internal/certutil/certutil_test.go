package certutil

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"strings"
	"testing"
)

const testCertPEM = `-----BEGIN CERTIFICATE-----
MIIDBTCCAe2gAwIBAgIUH1zJiy0bKoQXd2nZubWokLGpf5kwDQYJKoZIhvcNAQEL
BQAwEjEQMA4GA1UEAwwHVGVzdCBDQTAeFw0yNjA3MTExODMyMDBaFw0yNzA3MTEx
ODMyMDBaMBIxEDAOBgNVBAMMB1Rlc3QgQ0EwggEiMA0GCSqGSIb3DQEBAQUAA4IB
DwAwggEKAoIBAQCwdqOrF9Hd77u9TEfvvaq/7KuV3SqesWoIE1RgYLEP3Egu0BYn
ZxbBMR0ZT+f6aklWZscqYEJE+beO0fQptB3tSzUVsFaGcllzNUW13LJ31/dRcCCW
4jCBiPV6CO2bYYqoQU3zfCfSAAR1aQyg1HQNyZh+AzV0xvljA5I/tNFTx56W9qlc
6JPYVjAhg8v51AUkCYiB26Oo7sXokQ65S4LVlJJd6yxLY/vk71RPp/a3OT7PX0QK
dzl3OV6aiZPKzklKg3utvNXsj+v+6I6g7nuTcdPJfHLWvUpZihen61dXm5QV5DDv
rua7JXlGB4OpmER7fRWwX+OsP7DM1jLQ+byBAgMBAAGjUzBRMB0GA1UdDgQWBBS4
c5qu/KijkkY+nx1ofpMaOGcaajAfBgNVHSMEGDAWgBS4c5qu/KijkkY+nx1ofpMa
OGcaajAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQBdMTHuJzUY
mIhOH03Wm52KZU5xe6PL6FPHho52/tOIyl5VZwsEaKtjOiYj4ZqXnW6DugNpV9yK
s2H6jolg8ZoH5licn+dEjDuWTOn8A44TIm1UhpJTe1QCxM/cjU6VrEY92lkmR6qg
HXgUQfstPCc9GOY7QtIO43ubL4spBm+s5U6ERRhP5KLC4KGPQQIVq36IXaR9EFl/
YQJMg13DrFHljoabFc96n3V/5mBlrGSwrrHbSxtloStIBwvux6rN53NKA/p/nZQS
0uqxDvC4dW5q+G0ZT/4bHlBz7x0M/hcd8uGda1ChN9V6EV8jwDMlBWtQLHExfmWs
Awi6554msCQ1
-----END CERTIFICATE-----
`

func TestNormalizePEMFromSalesforce_Base64PEM(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(testCertPEM))

	got, err := NormalizePEMFromSalesforce(encoded)
	if err != nil {
		t.Fatalf("NormalizePEMFromSalesforce returned error: %v", err)
	}

	if got != testCertPEM {
		t.Fatalf("expected decoded PEM to be preserved\nwant: %q\n got: %q", testCertPEM, got)
	}
}

func TestNormalizePEMFromSalesforce_RawPEM(t *testing.T) {
	got, err := NormalizePEMFromSalesforce(testCertPEM)
	if err != nil {
		t.Fatalf("NormalizePEMFromSalesforce returned error: %v", err)
	}

	if got != testCertPEM {
		t.Fatalf("expected raw PEM to be preserved\nwant: %q\n got: %q", testCertPEM, got)
	}
}

func TestNormalizePEMFromSalesforce_Invalid(t *testing.T) {
	_, err := NormalizePEMFromSalesforce("not a certificate")
	if err == nil {
		t.Fatal("expected error for invalid certificate content")
	}
}

func TestLeafSHA256(t *testing.T) {
	block, _ := pem.Decode([]byte(testCertPEM))
	if block == nil {
		t.Fatal("test certificate PEM did not decode")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("test certificate did not parse: %v", err)
	}
	sum := sha256.Sum256(cert.Raw)
	want := strings.ToUpper(hex.EncodeToString(sum[:]))

	got, err := LeafSHA256(testCertPEM)
	if err != nil {
		t.Fatalf("LeafSHA256 returned error: %v", err)
	}

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestJWK(t *testing.T) {
	got, err := JWK(testCertPEM, "MyCert")
	if err != nil {
		t.Fatalf("JWK returned error: %v", err)
	}

	block, _ := pem.Decode([]byte(testCertPEM))
	cert, _ := x509.ParseCertificate(block.Bytes)
	n := base64.RawURLEncoding.EncodeToString(cert.PublicKey.(*rsa.PublicKey).N.Bytes())
	want := `{"kty":"RSA","kid":"MyCert","use":"sig","alg":"RS256","n":"` + n + `","e":"AQAB"}`
	if got != want {
		t.Fatalf("unexpected JWK\nwant: %s\n got: %s", want, got)
	}
}
