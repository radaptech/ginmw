package ginmw

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// A allowlist é o que impede qualquer site na internet de ler a resposta
// via credentials. Errar aqui é liberar demais (CSRF-like) ou de menos
// (produção quebrada) -- os dois silenciosos até alguém reclamar.
func TestCORSOrigemPermitida(t *testing.T) {
	gin.SetMode(gin.TestMode)

	casos := []struct {
		nome    string
		origem  string
		permite bool
	}{
		{"domínio exato", "https://radaptech.com.br", true},
		{"subdomínio do domínio", "https://app.radaptech.com.br", true},
		{"localhost sem porta", "http://localhost", true},
		{"localhost com porta", "http://localhost:3000", true},
		{"subdomínio .localhost", "http://app.localhost", true},
		{"domínio não relacionado", "https://evil.com", false},
		{"domínio parecido mas diferente", "https://radaptech.com.br.evil.com", false},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			r := gin.New()
			r.Use(CORS("radaptech.com.br"))
			r.GET("/", func(c *gin.Context) { c.Status(200) })

			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Origin", caso.origem)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			got := w.Header().Get("Access-Control-Allow-Origin")
			if caso.permite && got != caso.origem {
				t.Errorf("origem %q deveria ser permitida, Access-Control-Allow-Origin=%q", caso.origem, got)
			}
			if !caso.permite && got != "" {
				t.Errorf("origem %q não deveria ser permitida, mas veio Access-Control-Allow-Origin=%q", caso.origem, got)
			}
		})
	}
}

// Preflight sem 204 + os headers certos faz o browser nunca chegar a
// mandar o request de verdade (PUT/DELETE com header custom, por exemplo).
func TestCORSPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(CORS("radaptech.com.br"))
	r.PUT("/", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "https://radaptech.com.br")
	req.Header.Set("Access-Control-Request-Method", "PUT")
	req.Header.Set("Access-Control-Request-Headers", "X-Tenant-ID")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 204 {
		t.Fatalf("status do preflight = %d, esperado 204", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Error("preflight sem Access-Control-Allow-Credentials: true")
	}
}
