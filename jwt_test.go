package ginmw

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var jwtTestSecret = []byte("segredo-de-teste")

func assinar(t *testing.T, claims jwt.MapClaims, secret []byte) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("erro ao assinar token de teste: %v", err)
	}
	return s
}

func claimsCompletos() jwt.MapClaims {
	return jwt.MapClaims{
		"sub":      float64(7),
		"tenantId": float64(3),
		"perfil":   "tecnico",
		"exp":      time.Now().Add(time.Hour).Unix(),
	}
}

func rotaProtegida(mw gin.HandlerFunc) (*gin.Engine, *int64, *int64, *string) {
	var userID, tenantID int64
	var role string

	r := gin.New()
	r.Use(mw)
	r.GET("/", func(c *gin.Context) {
		userID, _ = UserID(c)
		tenantID, _ = TenantID(c)
		role, _ = Role(c)
		c.Status(200)
	})
	return r, &userID, &tenantID, &role
}

func TestJWTSemToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, _, _, _ := rotaProtegida(JWT(jwtTestSecret))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", w.Code)
	}
}

func TestJWTTokenClaimsIncompletos(t *testing.T) {
	gin.SetMode(gin.TestMode)

	casos := []struct {
		nome   string
		claims jwt.MapClaims
	}{
		{"sem sub", jwt.MapClaims{"tenantId": float64(3), "perfil": "tecnico", "exp": time.Now().Add(time.Hour).Unix()}},
		{"sem tenantId", jwt.MapClaims{"sub": float64(7), "perfil": "tecnico", "exp": time.Now().Add(time.Hour).Unix()}},
		{"sem perfil", jwt.MapClaims{"sub": float64(7), "tenantId": float64(3), "exp": time.Now().Add(time.Hour).Unix()}},
	}

	// O comportamento que importa: token incompleto NUNCA passa com contexto
	// parcial -- é a diferença de comportamento entre OS e SGE que motivou a lib.
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			r, _, _, _ := rotaProtegida(JWT(jwtTestSecret))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+assinar(t, c.claims, jwtTestSecret))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, esperado 401", w.Code)
			}
		})
	}
}

func TestJWTTokenExpirado(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, _, _, _ := rotaProtegida(JWT(jwtTestSecret))

	claims := claimsCompletos()
	claims["exp"] = time.Now().Add(-time.Hour).Unix()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+assinar(t, claims, jwtTestSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", w.Code)
	}
}

func TestJWTAssinaturaErrada(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, _, _, _ := rotaProtegida(JWT(jwtTestSecret))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+assinar(t, claimsCompletos(), []byte("outro-segredo")))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", w.Code)
	}
}

func TestJWTTokenValidoViaBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, userID, tenantID, role := rotaProtegida(JWT(jwtTestSecret))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+assinar(t, claimsCompletos(), jwtTestSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, esperado 200", w.Code)
	}
	if *userID != 7 || *tenantID != 3 || *role != "tecnico" {
		t.Fatalf("contexto = (userID=%d, tenantID=%d, role=%q), esperado (7, 3, tecnico)", *userID, *tenantID, *role)
	}
}

func TestJWTTokenValidoViaCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, userID, _, _ := rotaProtegida(JWT(jwtTestSecret))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: assinar(t, claimsCompletos(), jwtTestSecret)})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, esperado 200", w.Code)
	}
	if *userID != 7 {
		t.Fatalf("userID = %d, esperado 7", *userID)
	}
}

// WithRoleClaim é o que permite o SGE (claim "role") e o OS (claim
// "perfil") usarem a mesma lib sem reemitir token.
func TestJWTWithRoleClaim(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, _, _, role := rotaProtegida(JWT(jwtTestSecret, WithRoleClaim("role")))

	claims := jwt.MapClaims{
		"sub":      float64(1),
		"tenantId": float64(1),
		"role":     "super_admin",
		"exp":      time.Now().Add(time.Hour).Unix(),
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+assinar(t, claims, jwtTestSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, esperado 200", w.Code)
	}
	if *role != "super_admin" {
		t.Fatalf("role = %q, esperado super_admin", *role)
	}
}

// WithExtraClaim é best-effort: presente, vai pro contexto; ausente, não
// aborta a requisição (só não seta a chave).
func TestJWTWithExtraClaim(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var nome string
	r := gin.New()
	r.Use(JWT(jwtTestSecret, WithExtraClaim("nome", "user_nome")))
	r.GET("/", func(c *gin.Context) {
		nome = c.GetString("user_nome")
		c.Status(200)
	})

	t.Run("claim presente", func(t *testing.T) {
		claims := claimsCompletos()
		claims["nome"] = "Fulano"
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+assinar(t, claims, jwtTestSecret))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != 200 || nome != "Fulano" {
			t.Fatalf("status=%d nome=%q, esperado 200/Fulano", w.Code, nome)
		}
	})

	t.Run("claim ausente nao aborta", func(t *testing.T) {
		nome = "sobrou-do-teste-anterior"
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+assinar(t, claimsCompletos(), jwtTestSecret))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != 200 || nome != "" {
			t.Fatalf("status=%d nome=%q, esperado 200/\"\" (claim ausente, best-effort)", w.Code, nome)
		}
	})
}

func TestJWTSecretVazioEntraEmPanic(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("JWT com secret vazio deveria entrar em panic")
		}
	}()
	JWT(nil)
}
