package strava

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testClient(t *testing.T, app AppConfig, tok *TokenFile) (*Client, string) {
	t.Helper()
	dir := t.TempDir()
	appPath := filepath.Join(dir, "strava-app.json")
	tokenPath := filepath.Join(dir, "strava-tokens.json")
	raw, _ := json.Marshal(app)
	if err := os.WriteFile(appPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if tok != nil {
		raw, _ = json.Marshal(tok)
		if err := os.WriteFile(tokenPath, raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	c := New(appPath, tokenPath, "http://localhost:8080/api/strava/callback")
	c.pollWait = 0
	return c, dir
}

func TestStatusUnconfigured(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "missing.json"), filepath.Join(t.TempDir(), "tok.json"), "http://localhost:8080/api/strava/callback")
	st := c.GetStatus()
	if st.Configured || st.Connected {
		t.Fatalf("status = %+v", st)
	}
}

func TestStatusConnected(t *testing.T) {
	c, _ := testClient(t, AppConfig{ClientID: "id", ClientSecret: "sec"}, &TokenFile{
		AccessToken: "a", RefreshToken: "r", AthleteName: "Ailona G",
	})
	st := c.GetStatus()
	if !st.Configured || !st.Connected || st.Athlete != "Ailona G" {
		t.Fatalf("status = %+v", st)
	}
}

func TestAuthURLScopeAndRedirect(t *testing.T) {
	c, _ := testClient(t, AppConfig{ClientID: "99", ClientSecret: "sec"}, nil)
	u, err := c.AuthURL()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	if q.Get("client_id") != "99" {
		t.Fatalf("client_id %s", q.Get("client_id"))
	}
	if q.Get("scope") != "read,activity:write" {
		t.Fatalf("scope %s", q.Get("scope"))
	}
	if q.Get("redirect_uri") != "http://localhost:8080/api/strava/callback" {
		t.Fatalf("redirect %s", q.Get("redirect_uri"))
	}
	if q.Get("state") == "" {
		t.Fatal("missing state")
	}
}

func TestUploadMultipartAndIdempotent(t *testing.T) {
	var gotType, gotTrainer, gotAct, gotName, auth string
	var gotFile string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.Method == http.MethodPost {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse: %v", err)
			}
			gotType = r.FormValue("data_type")
			gotTrainer = r.FormValue("trainer")
			gotAct = r.FormValue("activity_type")
			gotName = r.FormValue("name")
			f, _, err := r.FormFile("file")
			if err == nil {
				b, _ := io.ReadAll(f)
				gotFile = string(b)
				f.Close()
			}
			_ = json.NewEncoder(w).Encode(uploadAPI{ID: 7, ActivityID: 42})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c, _ := testClient(t, AppConfig{ClientID: "id", ClientSecret: "sec"}, &TokenFile{
		AccessToken: "tok", RefreshToken: "ref", ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	c.uploadURL = srv.URL
	c.http = srv.Client()

	res := c.UploadTCX("GCN Sweet Spot", "https://youtu.be/x", []byte("<TrainingCenterDatabase/>"), 1000, 600)
	if !res.OK || res.ActivityID != 42 {
		t.Fatalf("upload %+v", res)
	}
	if auth != "Bearer tok" || gotType != "tcx" || gotTrainer != "true" || gotAct != "virtualride" {
		t.Fatalf("form type=%s trainer=%s act=%s auth=%s", gotType, gotTrainer, gotAct, auth)
	}
	if gotName != "GCN Sweet Spot" || !strings.Contains(gotFile, "TrainingCenterDatabase") {
		t.Fatalf("name=%s file=%s", gotName, gotFile)
	}

	again := c.UploadTCX("GCN Sweet Spot", "https://youtu.be/x", []byte("<TrainingCenterDatabase/>"), 1000, 600)
	if !again.OK || !again.Already || again.ActivityID != 42 {
		t.Fatalf("idempotent %+v", again)
	}
}

func TestUploadRejectsShortRide(t *testing.T) {
	c, _ := testClient(t, AppConfig{ClientID: "id", ClientSecret: "sec"}, &TokenFile{AccessToken: "a", RefreshToken: "r"})
	res := c.UploadTCX("x", "", []byte("tcx"), 1, 10)
	if res.OK || res.Error == "" {
		t.Fatalf("short ride %+v", res)
	}
}

func TestRefreshThenUpload(t *testing.T) {
	var sawBearer string
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.FormValue("grant_type") != "refresh_token" {
			http.Error(w, "bad grant", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "fresh", "refresh_token": "ref2", "expires_at": time.Now().Add(time.Hour).Unix(),
		})
	})
	mux.HandleFunc("/uploads", func(w http.ResponseWriter, r *http.Request) {
		sawBearer = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(uploadAPI{ID: 1, ActivityID: 9})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, _ := testClient(t, AppConfig{ClientID: "id", ClientSecret: "sec"}, &TokenFile{
		AccessToken: "old", RefreshToken: "ref", ExpiresAt: time.Now().Add(-time.Minute).Unix(),
	})
	c.tokenURL = srv.URL + "/token"
	c.uploadURL = srv.URL + "/uploads"
	c.http = srv.Client()
	res := c.UploadTCX("ride", "", []byte("tcx"), 5, 120)
	if !res.OK || res.ActivityID != 9 {
		t.Fatalf("upload %+v", res)
	}
	if sawBearer != "Bearer fresh" {
		t.Fatalf("bearer %s", sawBearer)
	}
}
