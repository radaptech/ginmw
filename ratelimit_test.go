package ginmw

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// taxa=0 (sem reposição) faz o teste ser determinístico: os primeiros
// `burst` requests passam, o resto bloqueia, sem precisar esperar tempo
// real passar.
func novoRouterLimitado(burst int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimit(rate.Limit(0), burst))
	r.GET("/", func(c *gin.Context) { c.Status(200) })
	return r
}

// É a garantia central do middleware: passa o burst, bloqueia. Sem isso a
// rota de login não tem proteção nenhuma contra força bruta.
func TestRateLimitBloqueiaAposBurst(t *testing.T) {
	r := novoRouterLimitado(2)
	ip := "1.2.3.4:1"

	for i := range 2 {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("request %d: status %d, esperado 200 (dentro do burst)", i+1, w.Code)
		}
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ip
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 429 {
		t.Fatalf("request após o burst: status %d, esperado 429", w.Code)
	}
}

// Um IP estourando o limite não pode travar os outros -- senão um único
// atacante de um IP derruba o login pra todo mundo (DoS de graça).
func TestRateLimitEIndependentePorIP(t *testing.T) {
	r := novoRouterLimitado(1)

	reqA := httptest.NewRequest("GET", "/", nil)
	reqA.RemoteAddr = "1.1.1.1:1"
	wA1 := httptest.NewRecorder()
	r.ServeHTTP(wA1, reqA)
	if wA1.Code != 200 {
		t.Fatalf("IP A, 1ª request: status %d, esperado 200", wA1.Code)
	}

	reqA2 := httptest.NewRequest("GET", "/", nil)
	reqA2.RemoteAddr = "1.1.1.1:1"
	wA2 := httptest.NewRecorder()
	r.ServeHTTP(wA2, reqA2)
	if wA2.Code != 429 {
		t.Fatalf("IP A, 2ª request: status %d, esperado 429 (estourou o burst)", wA2.Code)
	}

	reqB := httptest.NewRequest("GET", "/", nil)
	reqB.RemoteAddr = "2.2.2.2:1"
	wB := httptest.NewRecorder()
	r.ServeHTTP(wB, reqB)
	if wB.Code != 200 {
		t.Fatalf("IP B: status %d, esperado 200 (limiter separado do IP A)", wB.Code)
	}
}
