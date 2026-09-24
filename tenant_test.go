package ginmw

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// Só os ramos que abortam antes de chamar lookup -- por isso lookup é nil
// aqui, igual o TenantMiddleware(nil) do sistema-OS fazia com a query.
func TestTenantHeaderValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	casos := []struct {
		header string
		status int
	}{
		{"", http.StatusBadRequest},
		{"  ", http.StatusBadRequest},
		{"www", http.StatusForbidden},
		{"api", http.StatusForbidden},
	}

	for _, c := range casos {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		ctx.Request.Header.Set("X-Tenant-ID", c.header)

		Tenant(nil, errNaoAchou)(ctx)

		if w.Code != c.status {
			t.Errorf("header %q: status %d, esperado %d", c.header, w.Code, c.status)
		}
	}
}

var errNaoAchou = errors.New("não achou")

func TestTenantLookupSucesso(t *testing.T) {
	gin.SetMode(gin.TestMode)

	lookup := func(ctx context.Context, sub string) (int64, error) {
		if sub != "acme" {
			t.Fatalf("subdomínio passado ao lookup = %q, esperado acme", sub)
		}
		return 42, nil
	}

	r := gin.New()
	r.Use(Tenant(lookup, errNaoAchou))
	r.GET("/", func(c *gin.Context) {
		id, ok := TenantIDFromHeader(c)
		if !ok || id != 42 {
			t.Errorf("TenantIDFromHeader = (%d, %v), esperado (42, true)", id, ok)
		}
		c.Status(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Tenant-ID", "acme")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, esperado 200", w.Code)
	}
}

func TestTenantLookupNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	lookup := func(ctx context.Context, sub string) (int64, error) { return 0, errNaoAchou }

	r := gin.New()
	r.Use(Tenant(lookup, errNaoAchou))
	r.GET("/", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Tenant-ID", "inexistente")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, esperado 404", w.Code)
	}
}

// O erro real (que pode conter nome de driver, coluna, etc.) nunca pode
// vazar no corpo -- só vai pro log do servidor.
func TestTenantLookupErroGenericoNaoVaza(t *testing.T) {
	gin.SetMode(gin.TestMode)

	segredo := "connection refused em pg-primary-07.internal:5432"
	lookup := func(ctx context.Context, sub string) (int64, error) {
		return 0, errors.New(segredo)
	}

	r := gin.New()
	r.Use(Tenant(lookup, errNaoAchou))
	r.GET("/", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Tenant-ID", "acme")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, esperado 500", w.Code)
	}
	if strings.Contains(w.Body.String(), segredo) {
		t.Fatalf("corpo da resposta vazou o erro interno: %s", w.Body.String())
	}
}

func TestTenantIDFromHeaderSemContexto(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	if _, ok := TenantIDFromHeader(ctx); ok {
		t.Error("sem tenant no contexto deveria devolver false")
	}
}
