package calendar

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
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
	calendarScope = "https://www.googleapis.com/auth/calendar.readonly"
	calendarAPI   = "https://www.googleapis.com/calendar/v3"
	maxResponse   = 4 * 1024 * 1024
)

type event struct {
	Title  string
	Start  time.Time
	AllDay bool
}

type googleReader struct {
	path    string
	client  *http.Client
	apiURL  string
	now     func() time.Time
	mu      sync.Mutex
	token   string
	expires time.Time
	key     string
}

type eventsResponse struct {
	Items []struct {
		Status  string `json:"status"`
		Summary string `json:"summary"`
		Start   struct {
			Date     string `json:"date"`
			DateTime string `json:"dateTime"`
		} `json:"start"`
	} `json:"items"`
}

func newGoogleReader(path string) *googleReader {
	return &googleReader{
		path: path,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		apiURL: calendarAPI,
		now:    time.Now,
	}
}

func (r *googleReader) read(
	ctx context.Context, calendarID string, from, to time.Time,
) ([]event, error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return nil, fmt.Errorf("read Google credentials: %w", err)
	}
	credentials, key, err := parseCredentials(data)
	if err != nil {
		return nil, err
	}

	events, err := r.fetch(ctx, credentials, key, calendarID, from, to)
	if errors.Is(err, errUnauthorized) {
		// Google may revoke a token before its expiry; get a fresh one once.
		r.resetToken()
		events, err = r.fetch(ctx, credentials, key, calendarID, from, to)
	}
	return events, err
}

var errUnauthorized = errors.New("Google Calendar rejected access token")

func (r *googleReader) fetch(
	ctx context.Context, credentials *credentials, key *rsa.PrivateKey,
	calendarID string, from, to time.Time,
) ([]event, error) {
	token, err := r.accessToken(ctx, credentials, key)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf(
		"%s/calendars/%s/events",
		strings.TrimRight(r.apiURL, "/"),
		url.PathEscape(calendarID),
	)
	query := url.Values{}
	query.Set("timeMin", from.Format(time.RFC3339))
	query.Set("timeMax", to.Format(time.RFC3339))
	query.Set("singleEvents", "true")
	query.Set("orderBy", "startTime")
	endpoint += "?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Google Calendar request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf(
			"%w: %w", errUnauthorized, responseError("Google Calendar", resp),
		)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, responseError("Google Calendar", resp)
	}

	var payload eventsResponse
	reader := io.LimitReader(resp.Body, maxResponse)
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode Google Calendar response: %w", err)
	}

	return mapEvents(payload, from.Location()), nil
}

func (r *googleReader) accessToken(
	ctx context.Context, credentials *credentials, key *rsa.PrivateKey,
) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Round(0) drops the monotonic reading: on macOS it stops while the
	// computer sleeps, so a token would look valid long after it expired.
	now := r.now().Round(0)
	cacheKey := credentials.ClientEmail + credentials.PrivateKey
	if r.token != "" && r.key == cacheKey && now.Before(r.expires) {
		return r.token, nil
	}

	assertion, err := signedAssertion(credentials, key, now)
	if err != nil {
		return "", err
	}
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, credentials.TokenURI,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Google token request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", responseError("Google token", resp)
	}

	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	reader := io.LimitReader(resp.Body, maxResponse)
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode Google token response: %w", err)
	}
	if payload.AccessToken == "" {
		return "", errors.New("Google token response has no access_token")
	}

	r.token = payload.AccessToken
	r.key = cacheKey
	r.expires = now.Add(time.Duration(payload.ExpiresIn-60) * time.Second)
	return r.token, nil
}

func (r *googleReader) resetToken() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.token = ""
}

func signedAssertion(
	credentials *credentials, key *rsa.PrivateKey, now time.Time,
) (string, error) {
	header, err := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	})
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(map[string]interface{}{
		"iss":   credentials.ClientEmail,
		"scope": calendarScope,
		"aud":   credentials.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	if err != nil {
		return "", err
	}

	encode := base64.RawURLEncoding.EncodeToString
	unsigned := encode(header) + "." + encode(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(
		rand.Reader, key, crypto.SHA256, digest[:],
	)
	if err != nil {
		return "", err
	}
	return unsigned + "." + encode(signature), nil
}

func mapEvents(payload eventsResponse, location *time.Location) []event {
	result := make([]event, 0, len(payload.Items))
	for _, item := range payload.Items {
		if item.Status == "cancelled" || strings.TrimSpace(item.Summary) == "" {
			continue
		}

		value := event{Title: strings.TrimSpace(item.Summary)}
		var err error
		if item.Start.DateTime != "" {
			value.Start, err = time.Parse(time.RFC3339, item.Start.DateTime)
		} else {
			value.Start, err = time.ParseInLocation(
				"2006-01-02", item.Start.Date, location,
			)
			value.AllDay = true
		}
		if err == nil {
			result = append(result, value)
		}
	}
	return result
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
