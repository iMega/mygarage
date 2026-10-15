package googledrive

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	maxCredentialsSize = 1024 * 1024
	defaultAuthURI     = "https://accounts.google.com/o/oauth2/v2/auth"
	defaultTokenURI    = "https://oauth2.googleapis.com/token"
)

type client struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	AuthURI      string `json:"auth_uri"`
	TokenURI     string `json:"token_uri"`
}

type token struct {
	RefreshToken string `json:"refresh_token"`
}

func ValidateClient(data []byte) (string, error) {
	value, err := parseClient(data)
	if err != nil {
		return "", err
	}
	return value.ClientID, nil
}

func ReadClient(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return ValidateClient(data)
}

func SaveClient(path string, data []byte) (string, error) {
	if len(data) > maxCredentialsSize {
		return "", errors.New("client file is too large")
	}
	clientID, err := ValidateClient(data)
	if err != nil {
		return "", err
	}
	if err := writePrivate(path, data); err != nil {
		return "", err
	}
	return clientID, nil
}

func Connected(tokenPath string) bool {
	value, err := readToken(tokenPath)
	return err == nil && value.RefreshToken != ""
}

func readClient(path string) (*client, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Google Drive client: %w", err)
	}
	return parseClient(data)
}

func parseClient(data []byte) (*client, error) {
	var payload struct {
		Installed *client `json:"installed"`
		Web       *client `json:"web"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("invalid client JSON: %w", err)
	}
	if payload.Installed == nil {
		if payload.Web != nil {
			return nil, errors.New(
				"OAuth client must have Desktop app type",
			)
		}
		return nil, errors.New("client JSON must contain installed section")
	}

	value := payload.Installed
	value.ClientID = strings.TrimSpace(value.ClientID)
	value.ClientSecret = strings.TrimSpace(value.ClientSecret)
	value.AuthURI = strings.TrimSpace(value.AuthURI)
	value.TokenURI = strings.TrimSpace(value.TokenURI)
	if value.ClientID == "" || value.ClientSecret == "" {
		return nil, errors.New(
			"client JSON must contain client_id and client_secret",
		)
	}
	if value.AuthURI == "" {
		value.AuthURI = defaultAuthURI
	}
	if value.TokenURI == "" {
		value.TokenURI = defaultTokenURI
	}

	return value, nil
}

func readToken(path string) (*token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var value token
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("invalid Google Drive token: %w", err)
	}
	return &value, nil
}

func saveToken(path string, value token) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writePrivate(path, data)
}

func writePrivate(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}
