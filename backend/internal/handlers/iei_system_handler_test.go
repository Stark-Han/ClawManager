package handlers

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clawreef/internal/config"
	"clawreef/internal/models"
	"clawreef/internal/services"

	"github.com/gin-gonic/gin"
)

type fakeIEIInstanceService struct {
	*fakeWorkspaceHandlerInstanceService
	ownerInstances []models.Instance
}

func (s *fakeIEIInstanceService) GetSupportedByOwnerEmail(owner string, offset, limit int) ([]models.Instance, int, error) {
	start := min(offset, len(s.ownerInstances))
	end := min(start+limit, len(s.ownerInstances))
	return s.ownerInstances[start:end], len(s.ownerInstances), nil
}

func TestIEISystemEndpointsRequireSessionAndHideWrongOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := testIEIHandlerConfig()
	sso, err := services.NewIEISSOService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	owner := "owner@example.com"
	other := "other@example.com"
	instanceService := &fakeIEIInstanceService{
		fakeWorkspaceHandlerInstanceService: &fakeWorkspaceHandlerInstanceService{instances: map[int]*models.Instance{
			1: {ID: 1, UserID: 10, Owner: &owner, Name: "Owner Lite", Type: "openclaw", RuntimeType: "gateway", InstanceMode: "lite", Status: "running"},
			2: {ID: 2, UserID: 11, Owner: &other, Name: "Other Lite", Type: "openclaw", RuntimeType: "gateway", InstanceMode: "lite", Status: "running"},
			3: {ID: 3, UserID: 10, Owner: &owner, Name: "Owner WorkBuddy", Type: "workbuddy", RuntimeType: "desktop", RuntimeVariant: "linux", InstanceMode: "pro", Status: "running"},
			4: {ID: 4, UserID: 10, Owner: &owner, Name: "Windows WorkBuddy", Type: "workbuddy", RuntimeType: "desktop", RuntimeVariant: "windows", InstanceMode: "pro", Status: "running"},
		}},
		ownerInstances: []models.Instance{
			{ID: 1, UserID: 10, Owner: &owner, Name: "Owner Lite", Type: "openclaw", RuntimeType: "gateway", InstanceMode: "lite", Status: "running"},
			{ID: 3, UserID: 10, Owner: &owner, Name: "Owner WorkBuddy", Type: "workbuddy", RuntimeType: "desktop", RuntimeVariant: "linux", InstanceMode: "pro", Status: "running"},
		},
	}
	handler := NewIEISystemHandler(cfg, sso, instanceService, nil)
	router := gin.New()
	router.POST("/api/v1/ieisystem/session", handler.ExchangeSession)
	router.GET("/api/v1/ieisystem/instances", handler.ListInstances)
	router.GET("/api/v1/ieisystem/instances/:id", handler.GetInstance)

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/ieisystem/instances", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list status = %d, body = %s", unauthenticated.Code, unauthenticated.Body.String())
	}

	sessionCookie := exchangeIEITestSession(t, router, cfg, owner)
	listRecorder := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/ieisystem/instances", nil)
	listRequest.AddCookie(sessionCookie)
	router.ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK || !strings.Contains(listRecorder.Body.String(), "Owner Lite") ||
		!strings.Contains(listRecorder.Body.String(), "Owner WorkBuddy") ||
		!strings.Contains(listRecorder.Body.String(), `"runtime_variant":"linux"`) ||
		strings.Contains(listRecorder.Body.String(), "Other Lite") {
		t.Fatalf("owner list status = %d, body = %s", listRecorder.Code, listRecorder.Body.String())
	}

	workbuddyRecorder := httptest.NewRecorder()
	workbuddyRequest := httptest.NewRequest(http.MethodGet, "/api/v1/ieisystem/instances/3", nil)
	workbuddyRequest.AddCookie(sessionCookie)
	router.ServeHTTP(workbuddyRecorder, workbuddyRequest)
	if workbuddyRecorder.Code != http.StatusOK || !strings.Contains(workbuddyRecorder.Body.String(), "Owner WorkBuddy") {
		t.Fatalf("Linux WorkBuddy detail status = %d, body = %s", workbuddyRecorder.Code, workbuddyRecorder.Body.String())
	}

	windowsRecorder := httptest.NewRecorder()
	windowsRequest := httptest.NewRequest(http.MethodGet, "/api/v1/ieisystem/instances/4", nil)
	windowsRequest.AddCookie(sessionCookie)
	router.ServeHTTP(windowsRecorder, windowsRequest)
	if windowsRecorder.Code != http.StatusNotFound {
		t.Fatalf("Windows WorkBuddy detail status = %d, body = %s", windowsRecorder.Code, windowsRecorder.Body.String())
	}

	wrongOwnerRecorder := httptest.NewRecorder()
	wrongOwnerRequest := httptest.NewRequest(http.MethodGet, "/api/v1/ieisystem/instances/2", nil)
	wrongOwnerRequest.AddCookie(sessionCookie)
	router.ServeHTTP(wrongOwnerRecorder, wrongOwnerRequest)
	if wrongOwnerRecorder.Code != http.StatusNotFound {
		t.Fatalf("wrong-owner detail status = %d, body = %s", wrongOwnerRecorder.Code, wrongOwnerRecorder.Body.String())
	}
}

func TestIEIWorkspaceUsesIEISessionAndOwnerInsteadOfShareLink(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := testIEIHandlerConfig()
	sso, err := services.NewIEISSOService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	owner := "owner@example.com"
	other := "other@example.com"
	workspacePath := "/workspaces/user-10/instance-1"
	instanceService := &fakeIEIInstanceService{
		fakeWorkspaceHandlerInstanceService: &fakeWorkspaceHandlerInstanceService{instances: map[int]*models.Instance{
			1: {ID: 1, UserID: 10, Owner: &owner, Name: "Owner Lite", Type: "openclaw", RuntimeType: "gateway", InstanceMode: "lite", Status: "running", WorkspacePath: &workspacePath},
			2: {ID: 2, UserID: 11, Owner: &other, Name: "Other Lite", Type: "openclaw", RuntimeType: "gateway", InstanceMode: "lite", Status: "running", WorkspacePath: &workspacePath},
		}},
	}
	fileService := &fakeWorkspaceFileService{}
	workspaceHandler := NewWorkspaceFileHandler(instanceService, fileService)
	handler := NewIEISystemHandler(cfg, sso, instanceService, nil)
	handler.SetWorkspaceFileHandler(workspaceHandler)
	router := gin.New()
	router.POST("/api/v1/ieisystem/session", handler.ExchangeSession)
	router.GET("/api/v1/ieisystem/instances/:id/workspace/files", handler.ListWorkspace)

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/ieisystem/instances/1/workspace/files", nil))
	if unauthenticated.Code != http.StatusUnauthorized || fileService.listCalls != 0 {
		t.Fatalf("unauthenticated workspace status/calls = %d/%d", unauthenticated.Code, fileService.listCalls)
	}

	sessionCookie := exchangeIEITestSession(t, router, cfg, owner)
	wrongOwnerRecorder := httptest.NewRecorder()
	wrongOwnerRequest := httptest.NewRequest(http.MethodGet, "/api/v1/ieisystem/instances/2/workspace/files", nil)
	wrongOwnerRequest.AddCookie(sessionCookie)
	router.ServeHTTP(wrongOwnerRecorder, wrongOwnerRequest)
	if wrongOwnerRecorder.Code != http.StatusNotFound || fileService.listCalls != 0 {
		t.Fatalf("wrong-owner workspace status/calls = %d/%d", wrongOwnerRecorder.Code, fileService.listCalls)
	}

	ownerRecorder := httptest.NewRecorder()
	ownerRequest := httptest.NewRequest(http.MethodGet, "/api/v1/ieisystem/instances/1/workspace/files", nil)
	ownerRequest.AddCookie(sessionCookie)
	router.ServeHTTP(ownerRecorder, ownerRequest)
	if ownerRecorder.Code != http.StatusOK || !strings.Contains(ownerRecorder.Body.String(), "readme.md") {
		t.Fatalf("owner workspace status = %d, body = %s", ownerRecorder.Code, ownerRecorder.Body.String())
	}
	if fileService.listCalls != 1 || fileService.lastScope.InstanceID != 1 || fileService.lastScope.UserID != 10 || fileService.lastScope.WorkspacePath != workspacePath || fileService.lastScope.AuditActionPrefix != "iei_" {
		t.Fatalf("owner workspace calls/scope = %d/%#v", fileService.listCalls, fileService.lastScope)
	}
}

func TestProxyAccessTokenRequiresMatchingIEISession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := testIEIHandlerConfig()
	sso, err := services.NewIEISSOService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	location, _ := time.LoadLocation(cfg.Timezone)
	first, err := sso.ExchangeExternalToken(encryptIEITestToken(t, cfg, "owner@example.com+"+time.Now().In(location).Format("2006-01-02 15:04:05")))
	if err != nil {
		t.Fatal(err)
	}
	second, err := sso.ExchangeExternalToken(encryptIEITestToken(t, cfg, "owner@example.com+"+time.Now().In(location).Format("2006-01-02 15:04:05")))
	if err != nil {
		t.Fatal(err)
	}

	accessService := services.NewInstanceAccessService()
	defer accessService.Stop()
	access, err := accessService.GenerateBoundToken(
		1, 76, "openclaw", "/api/v1/instances/76/proxy/", "", 3001, time.Hour,
		ieiSystemSessionBinding(first.SessionID),
	)
	if err != nil {
		t.Fatal(err)
	}
	handler := &InstanceHandler{accessService: accessService, ieiSSOService: sso}

	requestContext := func(sessionToken string) *gin.Context {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/instances/76/proxy/", nil)
		ctx.Request.AddCookie(&http.Cookie{Name: "instance_access_76", Value: access.Token})
		if sessionToken != "" {
			ctx.Request.AddCookie(&http.Cookie{Name: ieiSystemSessionCookie, Value: sessionToken})
		}
		return ctx
	}

	if token, ok := handler.proxyAccessToken(requestContext(""), 76); ok || token != "" {
		t.Fatalf("IEI-bound access accepted without IEI session: %q/%v", token, ok)
	}
	if token, ok := handler.proxyAccessToken(requestContext(second.Token), 76); ok || token != "" {
		t.Fatalf("IEI-bound access accepted a different IEI session: %q/%v", token, ok)
	}
	if token, ok := handler.proxyAccessToken(requestContext(first.Token), 76); !ok || token != access.Token {
		t.Fatalf("IEI-bound access rejected its matching session: %q/%v", token, ok)
	}
}

func testIEIHandlerConfig() config.IEISystemConfig {
	return config.IEISystemConfig{
		Enabled:       true,
		AESKey:        "TESTKEY123456789",
		AESIV:         "0123456789ABCDEF",
		TokenTTL:      30 * time.Second,
		SessionTTL:    30 * time.Minute,
		SessionSecret: "test-only-iei-session-secret-at-least-32-bytes",
		Timezone:      "Asia/Shanghai",
	}
}

func exchangeIEITestSession(t *testing.T, router http.Handler, cfg config.IEISystemConfig, email string) *http.Cookie {
	t.Helper()
	location, _ := time.LoadLocation(cfg.Timezone)
	external := encryptIEITestToken(t, cfg, email+"+"+time.Now().In(location).Format("2006-01-02 15:04:05"))
	body, _ := json.Marshal(map[string]string{"token": external})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/ieisystem/session", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("session exchange status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == ieiSystemSessionCookie {
			return cookie
		}
	}
	t.Fatal("session exchange did not set IEI cookie")
	return nil
}

func encryptIEITestToken(t *testing.T, cfg config.IEISystemConfig, plaintext string) string {
	t.Helper()
	block, err := aes.NewCipher([]byte(cfg.AESKey))
	if err != nil {
		t.Fatal(err)
	}
	value := []byte(plaintext)
	padding := aes.BlockSize - len(value)%aes.BlockSize
	for range padding {
		value = append(value, byte(padding))
	}
	ciphertext := make([]byte, len(value))
	cipher.NewCBCEncrypter(block, []byte(cfg.AESIV)).CryptBlocks(ciphertext, value)
	return base64.StdEncoding.EncodeToString(ciphertext)
}
