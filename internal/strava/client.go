package strava

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	defaultAuthURL   = "https://www.strava.com/oauth/authorize"
	defaultTokenURL  = "https://www.strava.com/oauth/token"
	defaultUploadURL = "https://www.strava.com/api/v3/uploads"
	minUploadSec     = 30
)

type AppConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type TokenFile struct {
	AccessToken     string `json:"access_token"`
	RefreshToken    string `json:"refresh_token"`
	ExpiresAt       int64  `json:"expires_at"`
	AthleteID       int64  `json:"athlete_id"`
	AthleteName     string `json:"athlete_name"`
	LastUploadStart int64  `json:"last_upload_start,omitempty"`
	LastActivityID  int64  `json:"last_activity_id,omitempty"`
}

type Status struct {
	Configured bool   `json:"configured"`
	Connected  bool   `json:"connected"`
	Athlete    string `json:"athlete,omitempty"`
	Error      string `json:"error,omitempty"`
}

type UploadResult struct {
	OK         bool   `json:"ok"`
	Already    bool   `json:"already,omitempty"`
	ActivityID int64  `json:"activity_id,omitempty"`
	Error      string `json:"error,omitempty"`
}

type Client struct {
	mu          sync.Mutex
	appPath     string
	tokenPath   string
	redirectURI string
	oauthState  string
	http        *http.Client
	authURL     string
	tokenURL    string
	uploadURL   string
	pollWait    time.Duration
}

func New(appPath, tokenPath, redirectURI string) *Client {
	return &Client{
		appPath:     appPath,
		tokenPath:   tokenPath,
		redirectURI: redirectURI,
		http:        &http.Client{Timeout: 30 * time.Second},
		authURL:     defaultAuthURL,
		tokenURL:    defaultTokenURL,
		uploadURL:   defaultUploadURL,
		pollWait:    time.Second,
	}
}

func (c *Client) loadApp() (AppConfig, error) {
	cfg := AppConfig{
		ClientID:     strings.TrimSpace(os.Getenv("STRAVA_CLIENT_ID")),
		ClientSecret: strings.TrimSpace(os.Getenv("STRAVA_CLIENT_SECRET")),
	}
	if data, err := os.ReadFile(c.appPath); err == nil {
		var file AppConfig
		if json.Unmarshal(data, &file) == nil {
			if cfg.ClientID == "" {
				cfg.ClientID = strings.TrimSpace(file.ClientID)
			}
			if cfg.ClientSecret == "" {
				cfg.ClientSecret = strings.TrimSpace(file.ClientSecret)
			}
		}
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return cfg, errors.New("missing Strava client_id/secret (strava-app.json or STRAVA_CLIENT_ID/SECRET)")
	}
	return cfg, nil
}

func (c *Client) loadTokens() (TokenFile, error) {
	data, err := os.ReadFile(c.tokenPath)
	if err != nil {
		return TokenFile{}, err
	}
	var tok TokenFile
	if err := json.Unmarshal(data, &tok); err != nil {
		return TokenFile{}, err
	}
	return tok, nil
}

func (c *Client) saveTokens(tok TokenFile) error {
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.tokenPath, data, 0o600)
}

func (c *Client) GetStatus() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.loadApp(); err != nil {
		return Status{Configured: false, Error: err.Error()}
	}
	tok, err := c.loadTokens()
	if err != nil || tok.AccessToken == "" || tok.RefreshToken == "" {
		return Status{Configured: true, Connected: false}
	}
	return Status{Configured: true, Connected: true, Athlete: tok.AthleteName}
}

func (c *Client) AuthURL() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	app, err := c.loadApp()
	if err != nil {
		return "", err
	}
	state, err := randomState()
	if err != nil {
		return "", err
	}
	c.oauthState = state
	q := url.Values{
		"client_id":       {app.ClientID},
		"response_type":   {"code"},
		"redirect_uri":    {c.redirectURI},
		"approval_prompt": {"auto"},
		"scope":           {"read,activity:write"},
		"state":           {state},
	}
	return c.authURL + "?" + q.Encode(), nil
}

func (c *Client) Exchange(code, state string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.oauthState == "" || state != c.oauthState {
		return errors.New("invalid OAuth state")
	}
	c.oauthState = ""
	app, err := c.loadApp()
	if err != nil {
		return err
	}
	form := url.Values{
		"client_id":     {app.ClientID},
		"client_secret": {app.ClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	}
	tok, err := c.postToken(form)
	if err != nil {
		return err
	}
	return c.saveTokens(tok)
}

func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = os.Remove(c.tokenPath)
	return nil
}

func (c *Client) UploadTCX(name, description string, tcx []byte, startUnix int64, elapsedSec int) UploadResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elapsedSec < minUploadSec {
		return UploadResult{Error: "Ride too short to upload"}
	}
	if _, err := c.loadApp(); err != nil {
		return UploadResult{Error: err.Error()}
	}
	tok, err := c.loadTokens()
	if err != nil || tok.AccessToken == "" {
		return UploadResult{Error: "Strava is not connected"}
	}
	if startUnix != 0 && tok.LastUploadStart == startUnix && tok.LastActivityID != 0 {
		return UploadResult{OK: true, Already: true, ActivityID: tok.LastActivityID}
	}
	access, err := c.validAccessLocked(&tok)
	if err != nil {
		return UploadResult{Error: err.Error()}
	}
	activityID, err := c.postUpload(access, name, description, tcx)
	if err != nil {
		return UploadResult{Error: err.Error()}
	}
	tok.LastUploadStart = startUnix
	tok.LastActivityID = activityID
	_ = c.saveTokens(tok)
	return UploadResult{OK: true, ActivityID: activityID}
}

func (c *Client) validAccessLocked(tok *TokenFile) (string, error) {
	expiresSoon := tok.ExpiresAt > 0 && tok.ExpiresAt*1000 < time.Now().UnixMilli()+5*60*1000
	if !expiresSoon {
		return tok.AccessToken, nil
	}
	app, err := c.loadApp()
	if err != nil {
		return "", err
	}
	form := url.Values{
		"client_id":     {app.ClientID},
		"client_secret": {app.ClientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {tok.RefreshToken},
	}
	fresh, err := c.postToken(form)
	if err != nil {
		return "", err
	}
	if fresh.AthleteName == "" {
		fresh.AthleteName = tok.AthleteName
		fresh.AthleteID = tok.AthleteID
	}
	fresh.LastUploadStart = tok.LastUploadStart
	fresh.LastActivityID = tok.LastActivityID
	if err := c.saveTokens(fresh); err != nil {
		return "", err
	}
	*tok = fresh
	return tok.AccessToken, nil
}

func (c *Client) postToken(form url.Values) (TokenFile, error) {
	resp, err := c.http.PostForm(c.tokenURL, form)
	if err != nil {
		return TokenFile{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TokenFile{}, fmt.Errorf("strava token %d: %s", resp.StatusCode, truncate(body))
	}
	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    int64  `json:"expires_at"`
		Athlete      *struct {
			ID        int64  `json:"id"`
			Firstname string `json:"firstname"`
			Lastname  string `json:"lastname"`
		} `json:"athlete"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return TokenFile{}, err
	}
	if raw.AccessToken == "" || raw.RefreshToken == "" {
		return TokenFile{}, errors.New("strava token response missing tokens")
	}
	tok := TokenFile{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		ExpiresAt:    raw.ExpiresAt,
	}
	if raw.Athlete != nil {
		tok.AthleteID = raw.Athlete.ID
		tok.AthleteName = strings.TrimSpace(raw.Athlete.Firstname + " " + raw.Athlete.Lastname)
	}
	return tok, nil
}

type uploadAPI struct {
	ID         int64  `json:"id"`
	Status     string `json:"status"`
	Error      string `json:"error"`
	ActivityID int64  `json:"activity_id"`
}

func (c *Client) postUpload(access, name, description string, tcx []byte) (int64, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("data_type", "tcx")
	_ = w.WriteField("trainer", "true")
	_ = w.WriteField("activity_type", "virtualride")
	if name != "" {
		_ = w.WriteField("name", name)
	}
	if description != "" {
		_ = w.WriteField("description", description)
	}
	fw, err := w.CreateFormFile("file", "spin_workout.tcx")
	if err != nil {
		return 0, err
	}
	if _, err := fw.Write(tcx); err != nil {
		return 0, err
	}
	if err := w.Close(); err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPost, c.uploadURL, bytes.NewReader(buf.Bytes()))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("strava upload %d: %s", resp.StatusCode, truncate(body))
	}
	var u uploadAPI
	if err := json.Unmarshal(body, &u); err != nil {
		return 0, err
	}
	if u.Error != "" && u.ActivityID == 0 {
		return 0, errors.New(u.Error)
	}
	if u.ActivityID != 0 {
		return u.ActivityID, nil
	}
	if u.ID == 0 {
		return 0, errors.New("strava upload missing id")
	}
	return c.pollUpload(access, u.ID)
}

func (c *Client) pollUpload(access string, uploadID int64) (int64, error) {
	statusURL := strings.TrimRight(c.uploadURL, "/") + fmt.Sprintf("/%d", uploadID)
	for i := 0; i < 12; i++ {
		if c.pollWait > 0 {
			time.Sleep(c.pollWait)
		}
		req, err := http.NewRequest(http.MethodGet, statusURL, nil)
		if err != nil {
			return 0, err
		}
		req.Header.Set("Authorization", "Bearer "+access)
		resp, err := c.http.Do(req)
		if err != nil {
			return 0, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return 0, fmt.Errorf("strava upload status %d: %s", resp.StatusCode, truncate(body))
		}
		var last uploadAPI
		if err := json.Unmarshal(body, &last); err != nil {
			return 0, err
		}
		if last.Error != "" {
			return 0, errors.New(last.Error)
		}
		if last.ActivityID != 0 {
			return last.ActivityID, nil
		}
	}
	return 0, errors.New("strava is still processing the upload — try Post again in a minute")
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func truncate(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 240 {
		return s[:240]
	}
	return s
}
