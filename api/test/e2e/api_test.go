//go:build e2e

package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	apiPath               = "/api/v1"
	translationFeedbackID = "8bdf29a7-54d2-48ee-bd2b-c0ae79af6831"
)

type languageCapabilities struct {
	Source     []string `json:"source"`
	Target     []string `json:"target"`
	AutoDetect bool     `json:"auto_detect"`
}

type apiProcess struct {
	command *exec.Cmd
	done    chan struct{}
	output  *synchronizedBuffer

	mu  sync.Mutex
	err error
}

type synchronizedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *synchronizedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

type providerProxy struct {
	listener net.Listener
	ca       *x509.Certificate
	caKey    *rsa.PrivateKey

	certMu sync.Mutex
	certs  map[string]tls.Certificate

	connectionsMu sync.Mutex
	connections   map[net.Conn]struct{}
	waitGroup     sync.WaitGroup
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(data []byte) (int, error) {
	return c.reader.Read(data)
}

// TestAPIEndToEnd verifies the documented public lifecycle, authentication,
// validation, security boundaries, and response metadata against a real API.
func TestAPIEndToEnd(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)

	moduleRoot := apiModuleRoot(t)
	databaseURL := startPostgres(t, moduleRoot)
	proxyURL, certificateFile := startProviderProxy(t)
	port := availablePort(t)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	process := startAPI(t, moduleRoot, databaseURL, proxyURL, certificateFile, port)
	waitForAPI(t, process, baseURL+apiPath+"/languages")

	t.Run("reports API readiness", func(t *testing.T) {
		response := doRequest(t, newAPIClient(t), http.MethodGet, baseURL+"/health", "", "")
		defer response.Body.Close()

		requireStatus(t, response, http.StatusNoContent)
		requireEmptyBody(t, response.Body)
	})

	t.Run("publishes the configured language catalog", func(t *testing.T) {
		client := newAPIClient(t)
		response := doRequest(t, client, http.MethodGet, baseURL+apiPath+"/languages", "", "")
		defer response.Body.Close()

		requireStatus(t, response, http.StatusOK)
		var catalog map[string]languageCapabilities
		decodeJSON(t, response.Body, &catalog)

		chatGPT, ok := catalog["chatgpt"]
		if !ok {
			t.Fatal("language catalog does not contain chatgpt")
		}
		if !equalStrings(chatGPT.Source, []string{"English", "Spanish"}) ||
			!equalStrings(chatGPT.Target, []string{"English", "Spanish"}) || !chatGPT.AutoDetect {
			t.Fatalf("chatgpt capabilities = %#v; want configured English and Spanish source/target languages with auto-detection", chatGPT)
		}

		deepL, ok := catalog["deepl"]
		if !ok {
			t.Fatal("language catalog does not contain deepl")
		}
		if !equalStrings(deepL.Source, []string{"EN", "ES"}) ||
			!equalStrings(deepL.Target, []string{"EN", "ES"}) || !deepL.AutoDetect {
			t.Fatalf("deepl capabilities = %#v; want intercepted EN and ES source/target languages with auto-detection", deepL)
		}
	})

	t.Run("creates a session and accepts authenticated feedback", func(t *testing.T) {
		client := newAPIClient(t)
		createInstallation(t, client, baseURL)

		response := doRequest(t, client, http.MethodPut, baseURL+apiPath+"/analytics/feedback", "application/json", `{"rating":1,"text":"useful"}`)
		defer response.Body.Close()

		requireStatus(t, response, http.StatusNoContent)
		requireEmptyBody(t, response.Body)
		requireDocumentedResponseHeaders(t, response)
	})

	t.Run("accepts authenticated translation feedback", func(t *testing.T) {
		client := newAPIClient(t)
		createInstallation(t, client, baseURL)
		body := `{"translation_id":"` + translationFeedbackID + `","revision":1,"translator":"chatgpt","source_lang":"english","target_lang":"spanish","source_text":"hello","translation_text":"hola","assessment":"like"}`

		response := doRequest(t, client, http.MethodPut, baseURL+apiPath+"/analytics/feedback/translation", "application/json", body)
		defer response.Body.Close()

		requireStatus(t, response, http.StatusNoContent)
		requireEmptyBody(t, response.Body)
		requireDocumentedResponseHeaders(t, response)
	})

	t.Run("rejects invalid translation feedback", func(t *testing.T) {
		client := newAPIClient(t)
		createInstallation(t, client, baseURL)
		body := `{"translation_id":"` + translationFeedbackID + `","revision":1,"translator":"chatgpt","source_lang":"english","target_lang":"spanish","source_text":"hello","translation_text":"hola","assessment":"neutral"}`

		response := doRequest(t, client, http.MethodPut, baseURL+apiPath+"/analytics/feedback/translation", "application/json", body)
		defer response.Body.Close()

		requireStatus(t, response, http.StatusBadRequest)
		requireJSONResponse(t, response)
	})

	t.Run("rejects translation feedback without authentication", func(t *testing.T) {
		client := newAPIClient(t)
		body := `{"translation_id":"` + translationFeedbackID + `","revision":1,"translator":"chatgpt","source_lang":"english","target_lang":"spanish","source_text":"hello","translation_text":"hola","assessment":"like"}`

		response := doRequest(t, client, http.MethodPut, baseURL+apiPath+"/analytics/feedback/translation", "application/json", body)
		defer response.Body.Close()

		requireStatus(t, response, http.StatusUnauthorized)
		requireJSONResponse(t, response)
	})

	t.Run("rotates credentials and invalidates the prior session", func(t *testing.T) {
		client := newAPIClient(t)
		initialCookies := createInstallation(t, client, baseURL)

		response := doRequest(t, client, http.MethodPost, baseURL+apiPath+"/auth/handshake", "", "")
		defer response.Body.Close()

		requireStatus(t, response, http.StatusCreated)
		requireEmptyBody(t, response.Body)
		rotatedCookies := requireSessionCookies(t, response.Cookies())
		for _, name := range []string{"access_token", "refresh_token"} {
			if rotatedCookies[name].Value == initialCookies[name].Value {
				t.Fatalf("%s was not rotated", name)
			}
		}

		currentAccess := doRequest(t, client, http.MethodPut, baseURL+apiPath+"/analytics/feedback", "application/json", `{"rating":1}`)
		defer currentAccess.Body.Close()
		requireStatus(t, currentAccess, http.StatusNoContent)

		previousClient := newAPIClientWithCookies(t, baseURL, initialCookies)
		previousAccess := doRequest(t, previousClient, http.MethodPut, baseURL+apiPath+"/analytics/feedback", "application/json", `{"rating":1}`)
		defer previousAccess.Body.Close()
		requireStatus(t, previousAccess, http.StatusUnauthorized)

		replayedRefresh := doRequest(t, previousClient, http.MethodPost, baseURL+apiPath+"/auth/handshake", "", "")
		defer replayedRefresh.Body.Close()
		requireStatus(t, replayedRefresh, http.StatusUnauthorized)
	})

	t.Run("rejects feedback without authentication", func(t *testing.T) {
		client := newAPIClient(t)
		response := doRequest(t, client, http.MethodPut, baseURL+apiPath+"/analytics/feedback", "application/json", `{"rating":1}`)
		defer response.Body.Close()

		requireStatus(t, response, http.StatusUnauthorized)
		requireJSONResponse(t, response)
	})

	t.Run("rejects malformed feedback JSON", func(t *testing.T) {
		client := newAPIClient(t)
		createInstallation(t, client, baseURL)
		response := doRequest(t, client, http.MethodPut, baseURL+apiPath+"/analytics/feedback", "application/json", "{")
		defer response.Body.Close()

		requireStatus(t, response, http.StatusBadRequest)
		requireJSONResponse(t, response)
	})

	t.Run("rejects a request body over the configured limit", func(t *testing.T) {
		client := newAPIClient(t)
		createInstallation(t, client, baseURL)
		body := `{"rating":1,"text":"` + strings.Repeat("x", 300) + `"}`
		response := doRequest(t, client, http.MethodPut, baseURL+apiPath+"/analytics/feedback", "application/json", body)
		defer response.Body.Close()

		requireStatus(t, response, http.StatusRequestEntityTooLarge)
		requireJSONResponse(t, response)
	})
}

func apiModuleRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve e2e test location")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func startPostgres(t *testing.T, moduleRoot string) string {
	t.Helper()
	ctx := context.Background()
	migrations := filepath.Join(moduleRoot, "docs", "migrations")
	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("ai_translate_e2e"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.WithOrderedInitScripts(
			filepath.Join(migrations, "000001_initial_schema.up.sql"),
			filepath.Join(migrations, "000002_add_access_token_id.up.sql"),
			filepath.Join(migrations, "000003_add_translation_feedback.up.sql"),
		),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start PostgreSQL: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Errorf("terminate PostgreSQL container: %v", err)
		}
	})

	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get PostgreSQL connection string: %v", err)
	}
	return databaseURL
}

func startProviderProxy(t *testing.T) (string, string) {
	t.Helper()
	ca, caKey, caPEM := newCertificateAuthority(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for provider proxy: %v", err)
	}

	proxy := &providerProxy{
		listener:    listener,
		ca:          ca,
		caKey:       caKey,
		certs:       make(map[string]tls.Certificate),
		connections: make(map[net.Conn]struct{}),
	}
	go proxy.serve()
	t.Cleanup(proxy.close)

	certificateFile := filepath.Join(t.TempDir(), "provider-ca.pem")
	if err := os.WriteFile(certificateFile, caPEM, 0o600); err != nil {
		t.Fatalf("write provider CA certificate: %v", err)
	}
	return "http://" + listener.Addr().String(), certificateFile
}

func newCertificateAuthority(t *testing.T) (*x509.Certificate, *rsa.PrivateKey, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate provider CA key: %v", err)
	}
	serial, err := randomSerial()
	if err != nil {
		t.Fatalf("generate provider CA serial: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "AITranslate E2E Provider CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create provider CA certificate: %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse provider CA certificate: %v", err)
	}
	return certificate, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}

func (p *providerProxy) serve() {
	for {
		connection, err := p.listener.Accept()
		if err != nil {
			return
		}
		p.connectionsMu.Lock()
		p.connections[connection] = struct{}{}
		p.connectionsMu.Unlock()
		p.waitGroup.Add(1)
		go func() {
			defer p.waitGroup.Done()
			defer func() {
				p.connectionsMu.Lock()
				delete(p.connections, connection)
				p.connectionsMu.Unlock()
				_ = connection.Close()
			}()
			p.handle(connection)
		}()
	}
}

func (p *providerProxy) handle(connection net.Conn) {
	reader := bufio.NewReader(connection)
	connectRequest, err := http.ReadRequest(reader)
	if err != nil || connectRequest.Method != http.MethodConnect {
		return
	}
	if connectRequest.Body != nil {
		_ = connectRequest.Body.Close()
	}
	host := connectRequest.Host
	if splitHost, _, splitErr := net.SplitHostPort(host); splitErr == nil {
		host = splitHost
	}
	certificate, err := p.certificate(host)
	if err != nil {
		return
	}
	if _, err := io.WriteString(connection, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}

	tlsConnection := tls.Server(&bufferedConn{Conn: connection, reader: reader}, &tls.Config{
		Certificates: []tls.Certificate{certificate},
		MinVersion:   tls.VersionTLS12,
	})
	if err := tlsConnection.Handshake(); err != nil {
		return
	}
	tlsReader := bufio.NewReader(tlsConnection)
	for {
		request, err := http.ReadRequest(tlsReader)
		if err != nil {
			return
		}
		if request.Body != nil {
			_, _ = io.Copy(io.Discard, request.Body)
			_ = request.Body.Close()
		}
		response := interceptedProviderResponse(request)
		if err := response.Write(tlsConnection); err != nil {
			_ = response.Body.Close()
			return
		}
		_ = response.Body.Close()
		if request.Close {
			return
		}
	}
}

func (p *providerProxy) certificate(host string) (tls.Certificate, error) {
	p.certMu.Lock()
	defer p.certMu.Unlock()
	if certificate, ok := p.certs[host]; ok {
		return certificate, nil
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := randomSerial()
	if err != nil {
		return tls.Certificate{}, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, p.ca, &key.PublicKey, p.caKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	certificate := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	p.certs[host] = certificate
	return certificate, nil
}

func interceptedProviderResponse(request *http.Request) *http.Response {
	if request.Method == http.MethodGet && request.URL.Path == "/v2/languages" {
		body := `[{"language":"EN","name":"English","supports_formality":false},{"language":"ES","name":"Spanish","supports_formality":false}]`
		return providerJSONResponse(request, http.StatusOK, body)
	}
	return providerJSONResponse(request, http.StatusBadGateway, `{"message":"unexpected provider request"}`)
}

func providerJSONResponse(request *http.Request, status int, body string) *http.Response {
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	headers.Set("Content-Length", fmt.Sprintf("%d", len(body)))
	return &http.Response{
		StatusCode:    status,
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        headers,
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       request,
	}
}

func (p *providerProxy) close() {
	_ = p.listener.Close()
	p.connectionsMu.Lock()
	for connection := range p.connections {
		_ = connection.Close()
	}
	p.connectionsMu.Unlock()
	p.waitGroup.Wait()
}

func availablePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve API port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release API port: %v", err)
	}
	return port
}

func startAPI(t *testing.T, moduleRoot, databaseURL, proxyURL, certificateFile string, port int) *apiProcess {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "ai-translate-api")
	build := exec.Command("go", "build", "-o", binary, "./cmd/ai_translate")
	build.Dir = moduleRoot
	buildOutput, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build API: %v\n%s", err, buildOutput)
	}

	command := exec.Command(binary)
	command.Dir = moduleRoot
	command.Env = replaceEnvironment(os.Environ(), map[string]string{
		"APP_ENV":                       "test",
		"APP_VERSION":                   "v1",
		"LOG_LEVEL":                     "error",
		"PORT":                          fmt.Sprintf("%d", port),
		"CORS_ORIGINS":                  "http://localhost:5173",
		"HTTP_REQUEST_TIMEOUT":          "5s",
		"HTTP_WRITE_TIMEOUT":            "6s",
		"HTTP_MAX_BODY_BYTES":           "256",
		"HTTP_RATE_LIMIT":               "1000",
		"HTTP_RATE_BURST":               "1000",
		"DATABASE_URL":                  databaseURL,
		"DATABASE_MIN_IDLE_CONNECTIONS": "0",
		"JWT_SECRET":                    "01234567890123456789012345678901",
		"AUTH_SECURE_COOKIES":           "false",
		"OPENAI_API_KEY":                "e2e-test-key",
		"OPENAI_MODEL":                  "gpt-4.1-nano",
		"OPENAI_TRANSLATION_LANGUAGES":  "English,Spanish",
		"OPENAI_REQUEST_TIMEOUT":        "2s",
		"OPENAI_MAX_RETRIES":            "0",
		"DEEPL_API_KEYS":                "e2e-test-key:fx",
		"TRANSLATE_PROMPT":              "Translate the supplied text.",
		"INTERPRET_PROMPT":              "Interpret the supplied text.",
		"ANALYZE_PROMPT":                "Analyze the supplied text.",
		"MAX_TRANSLATION_CHARACTERS":    "10000",
		"HTTPS_PROXY":                   proxyURL,
		"HTTP_PROXY":                    proxyURL,
		"ALL_PROXY":                     "",
		"NO_PROXY":                      "127.0.0.1,localhost",
		"SSL_CERT_FILE":                 certificateFile,
	})
	output := new(synchronizedBuffer)
	command.Stdout = output
	command.Stderr = output
	if err := command.Start(); err != nil {
		t.Fatalf("start API: %v", err)
	}

	process := &apiProcess{command: command, done: make(chan struct{}), output: output}
	go func() {
		err := command.Wait()
		process.mu.Lock()
		process.err = err
		process.mu.Unlock()
		close(process.done)
	}()
	t.Cleanup(func() { stopAPI(t, process) })
	return process
}

func replaceEnvironment(current []string, replacements map[string]string) []string {
	result := make([]string, 0, len(current)+len(replacements))
	for _, entry := range current {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := replacements[name]; !replaced {
			result = append(result, entry)
		}
	}
	for name, value := range replacements {
		result = append(result, name+"="+value)
	}
	return result
}

func stopAPI(t *testing.T, process *apiProcess) {
	t.Helper()
	select {
	case <-process.done:
		return
	default:
	}
	if err := process.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Errorf("signal API process: %v", err)
		return
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-process.done:
	case <-timer.C:
		if err := process.command.Process.Kill(); err != nil {
			t.Errorf("kill API process after shutdown timeout: %v", err)
		}
		<-process.done
	}
}

func waitForAPI(t *testing.T, process *apiProcess, readinessURL string) {
	t.Helper()
	client := newAPIClient(t)
	deadline := time.NewTimer(20 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()

	for {
		request, err := http.NewRequest(http.MethodGet, readinessURL, nil)
		if err != nil {
			t.Fatalf("create readiness request: %v", err)
		}
		response, err := client.Do(request)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}

		select {
		case <-process.done:
			process.mu.Lock()
			processErr := process.err
			process.mu.Unlock()
			t.Fatalf("API exited before becoming ready: %v\n%s", processErr, process.output.String())
		case <-deadline.C:
			t.Fatalf("API did not become ready\n%s", process.output.String())
		case <-ticker.C:
		}
	}
}

func newAPIClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	return &http.Client{
		Jar:       jar,
		Timeout:   3 * time.Second,
		Transport: &http.Transport{Proxy: nil},
	}
}

func newAPIClientWithCookies(t *testing.T, baseURL string, cookies map[string]*http.Cookie) *http.Client {
	t.Helper()
	client := newAPIClient(t)
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse API base URL: %v", err)
	}
	values := make([]*http.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		values = append(values, cookie)
	}
	client.Jar.SetCookies(parsedURL, values)
	return client
}

func createInstallation(t *testing.T, client *http.Client, baseURL string) map[string]*http.Cookie {
	t.Helper()
	response := doRequest(t, client, http.MethodPost, baseURL+apiPath+"/users/new", "", "")
	defer response.Body.Close()

	requireStatus(t, response, http.StatusCreated)
	requireEmptyBody(t, response.Body)
	requireDocumentedResponseHeaders(t, response)
	return requireSessionCookies(t, response.Cookies())
}

func requireSessionCookies(t *testing.T, cookies []*http.Cookie) map[string]*http.Cookie {
	t.Helper()
	byName := make(map[string]*http.Cookie, len(cookies))
	for _, cookie := range cookies {
		byName[cookie.Name] = cookie
	}
	for _, name := range []string{"access_token", "refresh_token"} {
		cookie := byName[name]
		if cookie == nil {
			t.Fatalf("response does not contain %s cookie", name)
		}
		if !cookie.HttpOnly {
			t.Fatalf("%s cookie is not HttpOnly", name)
		}
	}
	return byName
}

func doRequest(t *testing.T, client *http.Client, method, target, contentType, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, target, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create %s %s request: %v", method, target, err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("send %s %s request: %v", method, target, err)
	}
	return response
}

func requireStatus(t *testing.T, response *http.Response, want int) {
	t.Helper()
	if response.StatusCode == want {
		return
	}
	body, _ := io.ReadAll(response.Body)
	t.Fatalf("status = %d, body = %q; want %d", response.StatusCode, body, want)
}

func requireEmptyBody(t *testing.T, body io.Reader) {
	t.Helper()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("response body = %q; want empty body", data)
	}
}

func requireDocumentedResponseHeaders(t *testing.T, response *http.Response) {
	t.Helper()
	for _, name := range []string{"X-Request-ID", "X-RateLimit-Limit", "X-RateLimit-Remaining"} {
		if response.Header.Get(name) == "" {
			t.Errorf("response does not contain documented %s header", name)
		}
	}
}

func requireJSONResponse(t *testing.T, response *http.Response) {
	t.Helper()
	if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q; want application/json", contentType)
	}
	var body map[string]any
	decodeJSON(t, response.Body, &body)
	if len(body) == 0 {
		t.Fatal("JSON error response is empty")
	}
}

func decodeJSON(t *testing.T, reader io.Reader, target any) {
	t.Helper()
	if err := json.NewDecoder(reader).Decode(target); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
