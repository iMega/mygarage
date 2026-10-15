package googledrive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
)

const (
	driveAPI    = "https://www.googleapis.com/drive/v3"
	uploadAPI   = "https://www.googleapis.com/upload/drive/v3"
	folderMime  = "application/vnd.google-apps.folder"
	folderName  = "myGarage backups"
	backupMime  = "application/vnd.sqlite3"
	listPageMax = "1000"
)

type session struct {
	connector *Connector
	token     string
	apiURL    string
	uploadURL string
}

// Upload stores the file in the backup folder and keeps only the newest
// files. It returns the folder ID, which may differ from the given one when
// the folder was missing and had to be created.
func (c *Connector) Upload(
	ctx context.Context, folderID, path, name string, keep int,
) (string, error) {
	accessToken, err := c.accessToken(ctx)
	if err != nil {
		return folderID, err
	}
	s := &session{
		connector: c,
		token:     accessToken,
		apiURL:    driveAPI,
		uploadURL: uploadAPI,
	}
	return s.upload(ctx, folderID, path, name, keep)
}

func (s *session) upload(
	ctx context.Context, folderID, path, name string, keep int,
) (string, error) {
	ensured, err := s.ensureFolder(ctx, folderID)
	if err != nil {
		return folderID, err
	}
	folderID = ensured
	if err := s.uploadFile(ctx, folderID, path, name); err != nil {
		return folderID, err
	}
	return folderID, s.rotate(ctx, folderID, keep)
}

func (s *session) ensureFolder(
	ctx context.Context, folderID string,
) (string, error) {
	if folderID != "" {
		var folder struct {
			Trashed bool `json:"trashed"`
		}
		endpoint := fmt.Sprintf(
			"%s/files/%s?fields=id,trashed",
			s.apiURL, url.PathEscape(folderID),
		)
		status, err := s.doJSON(ctx, http.MethodGet, endpoint, nil, &folder)
		if err == nil && !folder.Trashed {
			return folderID, nil
		}
		if err != nil && status != http.StatusNotFound {
			return "", err
		}
	}

	var created struct {
		ID string `json:"id"`
	}
	_, err := s.doJSON(
		ctx, http.MethodPost, s.apiURL+"/files?fields=id",
		map[string]string{"name": folderName, "mimeType": folderMime},
		&created,
	)
	if err != nil {
		return "", err
	}
	if created.ID == "" {
		return "", errors.New("Google Drive returned empty folder ID")
	}
	return created.ID, nil
}

// uploadFile uses a resumable session, so the size is not limited by the
// multipart upload limit.
func (s *session) uploadFile(
	ctx context.Context, folderID, path, name string,
) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}

	metadata, err := json.Marshal(map[string]interface{}{
		"name":     name,
		"parents":  []string{folderID},
		"mimeType": backupMime,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		s.uploadURL+"/files?uploadType=resumable",
		bytes.NewReader(metadata),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Type", backupMime)
	resp, err := s.connector.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Google Drive upload session: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return responseError("Google Drive upload session", resp)
	}
	location := resp.Header.Get("Location")
	if location == "" {
		return errors.New("Google Drive returned no upload location")
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodPut, location, file)
	if err != nil {
		return err
	}
	req.ContentLength = info.Size()
	req.Header.Set("Content-Type", backupMime)
	resp, err = s.connector.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Google Drive upload: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK &&
		resp.StatusCode != http.StatusCreated {
		return responseError("Google Drive upload", resp)
	}
	return nil
}

func (s *session) rotate(ctx context.Context, folderID string, keep int) error {
	query := url.Values{}
	query.Set(
		"q",
		fmt.Sprintf(
			"'%s' in parents and trashed = false and mimeType != '%s'",
			strings.ReplaceAll(folderID, "'", `\'`), folderMime,
		),
	)
	query.Set("fields", "files(id,name)")
	query.Set("pageSize", listPageMax)

	var list struct {
		Files []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"files"`
	}
	_, err := s.doJSON(
		ctx, http.MethodGet, s.apiURL+"/files?"+query.Encode(), nil, &list,
	)
	if err != nil {
		return err
	}
	if len(list.Files) <= keep {
		return nil
	}

	sort.Slice(list.Files, func(i, j int) bool {
		return list.Files[i].Name < list.Files[j].Name
	})
	for _, file := range list.Files[:len(list.Files)-keep] {
		endpoint := s.apiURL + "/files/" + url.PathEscape(file.ID)
		_, err := s.doJSON(ctx, http.MethodDelete, endpoint, nil, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *session) doJSON(
	ctx context.Context, method, endpoint string, body, target interface{},
) (int, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	}

	resp, err := s.connector.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("Google Drive request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.StatusCode, responseError("Google Drive", resp)
	}
	if target == nil {
		return resp.StatusCode, nil
	}
	reader = io.LimitReader(resp.Body, maxResponse)
	if err := json.NewDecoder(reader).Decode(target); err != nil {
		return resp.StatusCode, fmt.Errorf(
			"decode Google Drive response: %w", err,
		)
	}
	return resp.StatusCode, nil
}
