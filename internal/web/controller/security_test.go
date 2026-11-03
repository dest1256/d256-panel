package controller

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/crypto"
	"github.com/mhsanaei/3x-ui/v3/internal/web/global"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/panel"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

// fakeWeb satisfies global.WebServer so controllers that register background jobs can be constructed.
type fakeWeb struct{ c *cron.Cron }

func (f fakeWeb) GetCron() *cron.Cron   { return f.c }
func (fakeWeb) GetCtx() context.Context { return context.Background() }
func (fakeWeb) GetWSHub() any           { return nil }

// securityEngine wires the real API router (auth, scope, CSRF middleware and every controller) over a
// fresh database. /test-login signs a session in without a CSRF token, like a browser that logged in.
func securityEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previous := global.GetWebServer()
	global.SetWebServer(fakeWeb{c: cron.New()})
	t.Cleanup(func() { global.SetWebServer(previous) })
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
	engine := gin.New()
	engine.Use(sessions.Sessions("3x-ui", cookie.NewStore([]byte("security-test-secret"))))
	engine.GET("/test-login", func(c *gin.Context) {
		u, err := (&panel.UserService{}).GetFirstUser()
		if err != nil || session.SetLoginUser(c, u) != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})
	NewAPIController(engine.Group("/"))
	return engine
}

func token(t *testing.T, scope string) string {
	t.Helper()
	plain := "tok-" + scope
	if err := database.GetDB().Create(&model.ApiToken{
		Name: scope, Token: crypto.HashTokenSHA256(plain), Enabled: true, Scope: scope,
	}).Error; err != nil {
		t.Fatal(err)
	}
	return plain
}

func do(engine *gin.Engine, method, target string, body []byte, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	for k, v := range header {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

var newRoutes = []struct{ method, path string }{
	{"GET", "/panel/api/clientInsights/connections?email=a@x"},
	{"GET", "/panel/api/clientInsights/devices?email=a@x"},
	{"GET", "/panel/api/clientInsights/destinations?email=a@x"},
	{"GET", "/panel/api/clientInsights/traffic?email=a@x"},
	{"GET", "/panel/api/nodes/list"},
	{"POST", "/panel/api/nodes/create"},
	{"POST", "/panel/api/nodes/importXui/1"},
	{"POST", "/panel/api/nodes/enrollToken/1"},
}

func TestAnonymousCannotReachNewRoutes(t *testing.T) {
	engine := securityEngine(t)
	for _, r := range newRoutes {
		w := do(engine, r.method, r.path, nil, nil)
		if w.Code == http.StatusOK || strings.Contains(w.Body.String(), `"success":true`) {
			t.Errorf("%s %s answered an anonymous request: %d %s", r.method, r.path, w.Code, w.Body.String())
		}
	}
}

func TestRestrictedTokensCannotReadClientInsightsOrManageNodes(t *testing.T) {
	engine := securityEngine(t)
	for _, scope := range []string{model.ApiScopeMonitor, model.ApiScopeNodeSync} {
		tok := token(t, scope)
		for _, r := range newRoutes {
			w := do(engine, r.method, r.path, nil, map[string]string{"Authorization": "Bearer " + tok})
			if w.Code != http.StatusForbidden {
				t.Errorf("scope %s: %s %s = %d, want 403", scope, r.method, r.path, w.Code)
			}
		}
	}
}

func TestSessionPostWithoutCSRFTokenIsRejected(t *testing.T) {
	engine := securityEngine(t)
	login := do(engine, "GET", "/test-login", nil, nil)
	cookies := login.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie")
	}
	for _, path := range []string{"/panel/api/nodes/importXui/1", "/panel/api/nodes/create", "/panel/api/nodes/enrollToken/1"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("name=x&address=y"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, c := range cookies {
			req.AddCookie(c)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("POST %s with a session but no CSRF token = %d, want 403", path, w.Code)
		}
	}
}

func TestInsightsQueriesAreNotInjectable(t *testing.T) {
	engine := securityEngine(t)
	db := database.GetDB()
	db.Create(&model.ConnStatHourly{NodeId: 1, Email: "a@x", InboundTag: "t", Dest: "ok.example", Network: "tcp", Hour: 3600, Up: 1, Down: 1, Conns: 1})
	hdr := map[string]string{"Authorization": "Bearer " + token(t, model.ApiScopeAdmin)}

	payloads := []string{
		"x' OR '1'='1",
		"a@x'; DROP TABLE conn_stats_hourly; --",
		"%' UNION SELECT email FROM users --",
		`" OR ""="`,
	}
	for _, p := range payloads {
		for _, param := range []string{"email", "q"} {
			q := url.Values{"email": {"a@x"}, "period": {"30d"}}
			q.Set(param, p)
			w := do(engine, "GET", "/panel/api/clientInsights/destinations?"+q.Encode(), nil, hdr)
			if w.Code >= 500 {
				t.Errorf("%s=%q caused %d: %s", param, p, w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "password") {
				t.Errorf("%s=%q leaked other tables: %s", param, p, w.Body.String())
			}
		}
		w := do(engine, "GET", "/panel/api/clientInsights/destinations?email=a@x&period=30d&sort="+url.QueryEscape(p), nil, hdr)
		if w.Code >= 500 {
			t.Errorf("sort=%q caused %d", p, w.Code)
		}
	}
	var n int64
	if err := db.Model(&model.ConnStatHourly{}).Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("table damaged: n=%d err=%v", n, err)
	}
	// The wildcard characters in a search are literals: a lone % must not match everything.
	w := do(engine, "GET", "/panel/api/clientInsights/destinations?email=a@x&period=30d&q=%25", nil, hdr)
	if strings.Contains(w.Body.String(), "ok.example") {
		t.Error("% in a search acted as a wildcard")
	}
}

func TestCSVExportNeutralisesFormulasAndIsNotSniffable(t *testing.T) {
	engine := securityEngine(t)
	database.GetDB().Create(&model.ConnStatHourly{NodeId: 1, Email: "a@x", InboundTag: "t", Dest: "=HYPERLINK(1)", Network: "tcp", Hour: 3600, Up: 1, Down: 1, Conns: 1})
	hdr := map[string]string{"Authorization": "Bearer " + token(t, model.ApiScopeAdmin)}
	w := do(engine, "GET", "/panel/api/clientInsights/destinations?email=a@x&period=custom&from=0&to=7200&format=csv", nil, hdr)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "'=HYPERLINK(1)") || strings.Contains(w.Body.String(), "\n=HYPERLINK") {
		t.Errorf("formula not neutralised: %q", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("content type %q", ct)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("CSV must be served with nosniff")
	}
}

func TestStoredMarkupComesBackAsDataNotHTML(t *testing.T) {
	engine := securityEngine(t)
	database.GetDB().Create(&model.ConnStatHourly{NodeId: 1, Email: "a@x", InboundTag: "t", Dest: "<script>alert(1)</script>", Network: "tcp", Hour: 3600, Up: 1, Down: 1, Conns: 1})
	hdr := map[string]string{"Authorization": "Bearer " + token(t, model.ApiScopeAdmin)}
	w := do(engine, "GET", "/panel/api/clientInsights/destinations?email=a@x&period=custom&from=0&to=7200", nil, hdr)
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content type %q: markup must never be served as HTML", ct)
	}
	if strings.Contains(w.Body.String(), "<script>") {
		t.Errorf("markup was not escaped in the JSON body: %s", w.Body.String())
	}
}

func TestImportXuiRejectsNonDatabaseUploads(t *testing.T) {
	engine := securityEngine(t)
	database.GetDB().Create(&model.Node{Name: "n", Address: "n.example", Enable: true, Status: "pending"})
	hdr := map[string]string{"Authorization": "Bearer " + token(t, model.ApiScopeAdmin)}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("db", "../../../etc/passwd")
	_, _ = fw.Write([]byte("root:x:0:0:root:/root:/bin/sh\n"))
	_ = mw.Close()
	hdr["Content-Type"] = mw.FormDataContentType()
	w := do(engine, "POST", "/panel/api/nodes/importXui/1", buf.Bytes(), hdr)
	if strings.Contains(w.Body.String(), `"success":true`) {
		t.Errorf("a non-database upload was accepted: %s", w.Body.String())
	}
}
