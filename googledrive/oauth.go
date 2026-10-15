package googledrive

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	driveScope  = "https://www.googleapis.com/auth/drive.file"
	maxResponse = 4 * 1024 * 1024
	stateTTL    = 10 * time.Minute
)

type pending struct {
	verifier    string
	redirectURI string
	expires     time.Time
}

type Connector struct {
	clientPath string
	tokenPath  string
	httpClient *http.Client
	now        func() time.Time
	mu         sync.Mutex
	states     map[string]pending
}

func NewConnector(clientPath, tokenPath string) *Connector {
	return &Connector{
		clientPath: clientPath,
		tokenPath:  tokenPath,
		httpClient: &http.Client{Timeout: 2 * time.Minute},
		now:        time.Now,
		states:     map[string]pending{},
	}
}

// AuthURL starts the loopback OAuth flow for a Desktop app client.
func (c *Connector) AuthURL(redirectURI string) (string, error) {
	value, err := readClient(c.clientPath)
	if err != nil {
		return "", err
	}

	state, err := randomString()
	if err != nil {
		return "", err
	}
	verifier, err := randomString()
	if err != nil {
		return "", err
	}
	challenge := sha256.Sum256([]byte(verifier))

	c.mu.Lock()
	now := c.now()
	for key, item := range c.states {
		if now.After(item.expires) {
			delete(c.states, key)
		}
	}
	c.states[state] = pending{
		verifier:    verifier,
		redirectURI: redirectURI,
		expires:     now.Add(stateTTL),
	}
	c.mu.Unlock()

	query := url.Values{}
	query.Set("client_id", value.ClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("response_type", "code")
	query.Set("scope", driveScope)
	query.Set("access_type", "offline")
	query.Set("prompt", "consent")
	query.Set("state", state)
	query.Set(
		"code_challenge",
		base64.RawURLEncoding.EncodeToString(challenge[:]),
	)
	query.Set("code_challenge_method", "S256")

	return value.AuthURI + "?" + query.Encode(), nil
}

// Complete exchanges the authorization code and stores the refresh token.
func (c *Connector) Complete(ctx context.Context, state, code string) error {
	c.mu.Lock()
	item, ok := c.states[state]
	delete(c.states, state)
	c.mu.Unlock()
	if !ok || c.now().After(item.expires) {
		return errors.New("authorization request expired, try again")
	}
	if code == "" {
		return errors.New("authorization code is missing")
	}

	value, err := readClient(c.clientPath)
	if err != nil {
		return err
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", value.ClientID)
	form.Set("client_secret", value.ClientSecret)
	form.Set("redirect_uri", item.redirectURI)
	form.Set("code_verifier", item.verifier)

	payload, err := c.requestToken(ctx, value.TokenURI, form)
	if err != nil {
		return err
	}
	if payload.RefreshToken == "" {
		return errors.New("Google did not return refresh_token")
	}

	return saveToken(c.tokenPath, token{RefreshToken: payload.RefreshToken})
}

// Disconnect removes the stored refresh token.
func (c *Connector) Disconnect() error {
	if err := os.Remove(c.tokenPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (c *Connector) accessToken(ctx context.Context) (string, error) {
	value, err := readClient(c.clientPath)
	if err != nil {
		return "", err
	}
	stored, err := readToken(c.tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("Google Drive is not connected")
		}
		return "", err
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", stored.RefreshToken)
	form.Set("client_id", value.ClientID)
	form.Set("client_secret", value.ClientSecret)

	payload, err := c.requestToken(ctx, value.TokenURI, form)
	if err != nil {
		return "", err
	}
	if payload.AccessToken == "" {
		return "", errors.New("Google token response has no access_token")
	}
	return payload.AccessToken, nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func (c *Connector) requestToken(
	ctx context.Context, tokenURI string, form url.Values,
) (*tokenResponse, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, tokenURI, strings.NewReader(form.Encode()),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Google token request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, responseError("Google token", resp)
	}

	var payload tokenResponse
	reader := io.LimitReader(resp.Body, maxResponse)
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode Google token response: %w", err)
	}
	return &payload, nil
}

func randomString() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func responseError(name string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(resp.StatusCode)
	}
	return fmt.Errorf(
		"%s returned %s: %s",
		name, strconv.Itoa(resp.StatusCode), message,
	)
}
