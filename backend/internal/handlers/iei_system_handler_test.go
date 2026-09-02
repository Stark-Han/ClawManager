package handlers

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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
	restartCalls   []int
	restartErr     error
	resetCalls     []int
	resetErr       error
}

func (s *fakeIEIInstanceService) GetSupportedByOwnerEmail(owner string, offset, limit int) ([]models.Instance, int, error) {
	start := min(offset, len(s.ownerInstances))
	end := min(start+limit, len(s.ownerInstances))
	return s.ownerInstances[start:end], len(s.ownerInstances), nil
}

func (s *fakeIEIInstanceService) Restart(instanceID int) error {
	s.restartCalls = append(s.restartCalls, instanceID)
	return s.restartErr
}

func (s *fakeIEIInstanceService) Reset(instanceID int) error {
	s.resetCalls = append(s.resetCalls, instanceID)
	return s.resetErr
}

func TestIEISystemRestartRequiresSessionOwnerAndRunningInstance(t *testing.T) {
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
			3: {ID: 3, UserID: 10, Owner: &owner, Name: "Stopped Pro", Type: "workbuddy", RuntimeType: "desktop", RuntimeVariant: "linux", InstanceMode: "pro", Status: "stopped"},
		}},
	}
	handler := NewIEISystemHandler(cfg, sso, instanceService, nil)
	router := gin.New()
	router.POST("/api/v1/ieisystem/session", handler.ExchangeSession)
	router.POST("/api/v1/ieisystem/instances/:id/restart", handler.RestartInstance)

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodPost, "/api/v1/ieisystem/instances/1/restart", nil))
	if unauthenticated.Code != http.StatusUnauthorized || len(instanceService.restartCalls) != 0 {
		t.Fatalf("unauthenticated restart status/calls = %d/%v", unauthenticated.Code, instanceService.restartCalls)
	}

	sessionCookie := exchangeIEITestSession(t, router, cfg, owner)
	wrongOwner := httptest.NewRequest(http.MethodPost, "/api/v1/ieisystem/instances/2/restart", nil)
	wrongOwner.AddCookie(sessionCookie)
	wrongOwnerRecorder := httptest.NewRecorder()
	router.ServeHTTP(wrongOwnerRecorder, wrongOwner)
	if wrongOwnerRecorder.Code != http.StatusNotFound || len(instanceService.restartCalls) != 0 {
		t.Fatalf("wrong-owner restart status/calls = %d/%v", wrongOwnerRecorder.Code, instanceService.restartCalls)
	}

	stopped := httptest.NewRequest(http.MethodPost, "/api/v1/ieisystem/instances/3/restart", nil)
	stopped.AddCookie(sessionCookie)
	stoppedRecorder := httptest.NewRecorder()
	router.ServeHTTP(stoppedRecorder, stopped)
	if stoppedRecorder.Code != http.StatusConflict || len(instanceService.restartCalls) != 0 {
		t.Fatalf("stopped restart status/calls = %d/%v", stoppedRecorder.Code, instanceService.restartCalls)
	}

	owned := httptest.NewRequest(http.MethodPost, "/api/v1/ieisystem/instances/1/restart", nil)
	owned.AddCookie(sessionCookie)
	ownedRecorder := httptest.NewRecorder()
	router.ServeHTTP(ownedRecorder, owned)
	if ownedRecorder.Code != http.StatusAccepted || len(instanceService.restartCalls) != 1 || instanceService.restartCalls[0] != 1 {
		t.Fatalf("owner restart status/calls = %d/%v, body = %s", ownedRecorder.Code, instanceService.restartCalls, ownedRecorder.Body.String())
	}

	instanceService.restartErr = errors.New("kubernetes restart failed")
	serviceFailure := httptest.NewRequest(http.MethodPost, "/api/v1/ieisystem/instances/1/restart", nil)
	serviceFailure.AddCookie(sessionCookie)
	serviceFailureRecorder := httptest.NewRecorder()
	router.ServeHTTP(serviceFailureRecorder, serviceFailure)
	if serviceFailureRecorder.Code != http.StatusServiceUnavailable || strings.Contains(serviceFailureRecorder.Body.String(), "kubernetes") {
		t.Fatalf("restart failure status/body = %d/%s", serviceFailureRecorder.Code, serviceFailureRecorder.Body.String())
	}
}

func TestIEISystemResetRequiresOwnerAndAcceptsRecoverableStates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := testIEIHandlerConfig()
	sso, err := services.NewIEISSOService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	owner := "owner@example.com"
	other := "other@example.com"
	instanceService := &fakeIEIInstanceService{fakeWorkspaceHandlerInstanceService: &fakeWorkspaceHandlerInstanceService{instances: map[int]*models.Instance{
		1: {ID: 1, UserID: 10, Owner: &owner, Name: "Owner Pro", Type: "deepseek-harness", RuntimeType: "desktop", InstanceMode: "pro", Status: "running"},
		2: {ID: 2, UserID: 10, Owner: &owner, Name: "Owner Error", Type: "openclaw", RuntimeType: "gateway", InstanceMode: "lite", Status: "error"},
		3: {ID: 3, UserID: 11, Owner: &other, Name: "Other Pro", Type: "deepseek-harness", RuntimeType: "desktop", InstanceMode: "pro", Status: "running"},
		4: {ID: 4, UserID: 10, Owner: &owner, Name: "Creating", Type: "hermes", RuntimeType: "gateway", InstanceMode: "lite", Status: "creating"},
	}}}
	handler := NewIEISystemHandler(cfg, sso, instanceService, nil)
	router := gin.New()
	router.POST("/api/v1/ieisystem/session", handler.ExchangeSession)
	router.POST("/api/v1/ieisystem/instances/:id/reset", handler.ResetInstance)
	cookie := exchangeIEITestSession(t, router, cfg, owner)

	for _, id := range []int{1, 2} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ieisystem/instances/"+strconv.Itoa(id)+"/reset", nil)
		req.AddCookie(cookie)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("reset %d status/body = %d/%s", id, recorder.Code, recorder.Body.String())
		}
	}
	if len(instanceService.resetCalls) != 2 || instanceService.resetCalls[0] != 1 || instanceService.resetCalls[1] != 2 {
		t.Fatalf("reset calls = %v", instanceService.resetCalls)
	}

	for id, want := range map[int]int{3: http.StatusNotFound, 4: http.StatusConflict} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ieisystem/instances/"+strconv.Itoa(id)+"/reset", nil)
		req.AddCookie(cookie)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		if recorder.Code != want {
			t.Fatalf("reset %d status/body = %d/%s, want %d", id, recorder.Code, recorder.Body.String(), want)
		}
	}
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

func TestIEISystemRefreshSessionUsesExistingLocalSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := testIEIHandlerConfig()
	sso, err := services.NewIEISSOService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewIEISystemHandler(cfg, sso, &fakeIEIInstanceService{}, nil)
	router := gin.New()
	router.POST("/api/v1/ieisystem/session", handler.ExchangeSession)
	router.POST("/api/v1/ieisystem/session/refresh", handler.RefreshSession)

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodPost, "/api/v1/ieisystem/session/refresh", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated refresh status = %d, body = %s", unauthenticated.Code, unauthenticated.Body.String())
	}

	sessionCookie := exchangeIEITestSession(t, router, cfg, "owner@example.com")
	refreshed := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/ieisystem/session/refresh", nil)
	request.AddCookie(sessionCookie)
	router.ServeHTTP(refreshed, request)
	if refreshed.Code != http.StatusOK || !strings.Contains(refreshed.Body.String(), "owner@example.com") {
		t.Fatalf("refresh status/body = %d/%s", refreshed.Code, refreshed.Body.String())
	}
	if !strings.Contains(refreshed.Header().Get("Set-Cookie"), ieiSystemSessionCookie+"=") {
		t.Fatalf("refresh did not rotate the local session cookie: %s", refreshed.Header().Get("Set-Cookie"))
	}
}

func TestIEISystemDedicatedRuntimeAccessReturnsTokenBootstrapURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CLAWMANAGER_DEEPSEEK_HARNESS_PUBLIC_URL_TEMPLATE", "https://deepseek-harness-{instance_id}.runtime.example.test/")
	cfg := testIEIHandlerConfig()
	sso, err := services.NewIEISSOService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	owner := "owner@example.com"
	instanceService := &fakeIEIInstanceService{
		fakeWorkspaceHandlerInstanceService: &fakeWorkspaceHandlerInstanceService{instances: map[int]*models.Instance{
			28: {ID: 28, UserID: 10, Owner: &owner, Name: "Owner DSH", Type: services.RuntimeTypeDeepSeekHarness, RuntimeType: "gateway", InstanceMode: "lite", Status: "running"},
		}},
	}
	accessService := services.NewInstanceAccessService()
	defer accessService.Stop()
	instanceHandler := &InstanceHandler{
		accessService: accessService,
		proxyService:  services.NewInstanceProxyService(accessService),
	}
	handler := NewIEISystemHandler(cfg, sso, instanceService, instanceHandler)
	router := gin.New()
	router.POST("/api/v1/ieisystem/session", handler.ExchangeSession)
	router.POST("/api/v1/ieisystem/instances/:id/access", handler.GenerateInstanceAccess)

	sessionCookie := exchangeIEITestSession(t, router, cfg, owner)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/ieisystem/instances/28/access", nil)
	request.AddCookie(sessionCookie)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("access status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data struct {
			AccessURL string `json:"access_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(response.Data.AccessURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Host != "deepseek-harness-28.runtime.example.test" || parsed.Query().Get("token") == "" {
		t.Fatalf("dedicated access URL did not contain its token bootstrap: %q", response.Data.AccessURL)
	}
	if !accessService.IsInstanceAccessToken(parsed.Query().Get("token")) {
		t.Fatal("dedicated access URL token is not an instance-access capability")
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

	dedicatedAccess, err := accessService.GenerateBoundToken(
		1, 76, services.RuntimeTypeDeepSeekHarness, "https://deepseek-harness-76.runtime.example.test/", "", 3001, time.Hour,
		ieiSystemSessionBinding(first.SessionID),
	)
	if err != nil {
		t.Fatal(err)
	}
	dedicatedContext := func(host, queryToken, cookieToken string) (*gin.Context, *httptest.ResponseRecorder) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		requestURL := "/api/v1/instances/76/proxy/"
		if queryToken != "" {
			requestURL += "?token=" + url.QueryEscape(queryToken)
		}
		ctx.Request = httptest.NewRequest(http.MethodGet, requestURL, nil)
		ctx.Request.Host = host
		ctx.Request.Header.Set(services.DedicatedRuntimeOriginHeader, services.RuntimeTypeDeepSeekHarness)
		if cookieToken != "" {
			ctx.Request.AddCookie(&http.Cookie{Name: "instance_access_76", Value: cookieToken})
		}
		return ctx, recorder
	}
	wrongHostContext, _ := dedicatedContext("attacker.example.test", dedicatedAccess.Token, "")
	if token, ok := handler.proxyAccessToken(wrongHostContext, 76); ok || token != "" {
		t.Fatalf("IEI-bound dedicated access accepted the wrong host: %q/%v", token, ok)
	}

	staleAccess, err := accessService.GenerateBoundToken(
		1, 76, services.RuntimeTypeDeepSeekHarness, "https://deepseek-harness-76.runtime.example.test/", "", 3001, -time.Second,
		ieiSystemSessionBinding(first.SessionID),
	)
	if err != nil {
		t.Fatal(err)
	}
	dedicated, dedicatedRecorder := dedicatedContext(
		"deepseek-harness-76.runtime.example.test:30443", dedicatedAccess.Token, staleAccess.Token,
	)
	if token, ok := handler.proxyAccessToken(dedicated, 76); !ok || token != dedicatedAccess.Token {
		t.Fatalf("fresh IEI query capability did not replace stale dedicated cookie: %q/%v", token, ok)
	}
	setCookie := dedicatedRecorder.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, "instance_access_76="+dedicatedAccess.Token) ||
		!strings.Contains(setCookie, "Path=/") || !strings.Contains(setCookie, "SameSite=None") {
		t.Fatalf("dedicated IEI access cookie was not promoted: %s", setCookie)
	}
	refreshContext, _ := dedicatedContext("deepseek-harness-76.runtime.example.test:30443", "", "")
	refreshContext.Request.URL.Path = "/api/v1/instances/76/proxy" + dedicatedAccessRefreshPath
	refreshContext.Request.Header.Set(dedicatedAccessRefreshHeader, dedicatedAccess.Token)
	refreshToken, refreshOK := handler.proxyAccessToken(refreshContext, 76)
	if !refreshOK || refreshToken != dedicatedAccess.Token {
		t.Fatalf("dedicated access refresh header was rejected: %q/%v", refreshToken, refreshOK)
	}
	if !handler.isDedicatedAccessRefreshRequest(refreshContext, 76, refreshToken) {
		t.Fatal("dedicated access refresh endpoint rejected a valid bound token")
	}
	wrongPathContext, _ := dedicatedContext("deepseek-harness-76.runtime.example.test:30443", "", "")
	wrongPathContext.Request.URL.Path = "/api/v1/instances/76/proxy/not-refresh" + dedicatedAccessRefreshPath
	wrongPathContext.Request.Header.Set(dedicatedAccessRefreshHeader, dedicatedAccess.Token)
	if wrongPathToken, wrongPathOK := handler.proxyAccessToken(wrongPathContext, 76); wrongPathOK {
		t.Fatalf("internal refresh header escaped its exact path: %q", wrongPathToken)
	}
}

func testIEIHandlerConfig() config.IEISystemConfig {
	return config.IEISystemConfig{
		Enabled:       true,
		AESKey:        "TESTKEY123456789",
		AESIV:         "0123456789ABCDEF",
		TokenTTL:      24 * time.Hour,
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
