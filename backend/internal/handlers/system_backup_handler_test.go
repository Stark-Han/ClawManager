package handlers

import (
	"database/sql"
	"net/http/httptest"
	"strings"
	"testing"

	"clawreef/internal/systembackup"
	"github.com/gin-gonic/gin"
)

func TestBackupBodyRejectsAmbiguousDuplicateKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{`{"task_type":"backup","task_type":"drill"}`, `{"task_type":"backup","TASK_TYPE":"drill"}`} {
		r := gin.New()
		r.POST("/", func(c *gin.Context) {
			var v systembackup.CreateTaskRequest
			if backupBody(c, &v) {
				c.Status(204)
			}
		})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatalf("ambiguous request accepted: %d", w.Code)
		}
	}
}

func TestBackupPermissionChecksPrecedeDatabaseAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		actor                    int
		role, method, path, body string
		want                     int
	}{{0, "", "GET", "/overview", "", 401}, {7, "viewer", "POST", "/tasks", `{}`, 403}, {7, "operator", "PUT", "/config", `{}`, 403}, {7, "config_admin", "POST", "/tasks", `{}`, 403}, {7, "operator", "POST", "/tasks", `{"unexpected":true}`, 400}, {7, "operator", "POST", "/tasks", `{} {}`, 400}} {
		t.Run(tc.role+tc.path+tc.body, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) { c.Set("userID", tc.actor) })
			h := NewSystemBackupHandler(systembackup.Store{DB: &sql.DB{}, InstallationID: "test"}, systembackup.RoleBindings{tc.actor: tc.role})
			h.Register(r.Group(""))
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
