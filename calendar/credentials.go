package calendar

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
)

const maxCredentialsSize = 1024 * 1024

type credentials struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

func ValidateCredentials(data []byte) (string, error) {
	credentials, _, err := parseCredentials(data)
	if err != nil {
		return "", err
	}
	return credentials.ClientEmail, nil
}

func ReadCredentials(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return ValidateCredentials(data)
}

func SaveCredentials(path string, data []byte) (string, error) {
	if len(data) > maxCredentialsSize {
		return "", errors.New("credentials file is too large")
	}
	email, err := ValidateCredentials(data)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0600); err != nil {
		return "", err
	}
	return email, nil
}

func parseCredentials(data []byte) (*credentials, *rsa.PrivateKey, error) {
	var value credentials
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, nil, fmt.Errorf("invalid credentials JSON: %w", err)
	}
	value.ClientEmail = strings.TrimSpace(value.ClientEmail)
	value.TokenURI = strings.TrimSpace(value.TokenURI)
	if value.ClientEmail == "" || value.PrivateKey == "" {
		return nil, nil, errors.New(
			"credentials must contain client_email and private_key",
		)
	}
	if value.TokenURI == "" {
		value.TokenURI = "https://oauth2.googleapis.com/token"
	}

	block, _ := pem.Decode([]byte(value.PrivateKey))
	if block == nil {
		return nil, nil, errors.New("credentials contain invalid private_key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse private_key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, errors.New("private_key must be RSA")
	}

	return &value, key, nil
}
