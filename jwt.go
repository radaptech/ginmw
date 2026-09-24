package ginmw

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Chaves do contexto do Gin. Em código de produção, leia pelos getters
// (UserID, TenantID, Role) -- as constantes são exportadas só pra teste
// conseguir simular um contexto autenticado sem passar por um JWT de
// verdade (veja o padrão usado nos *_test.go dos consumidores).
const (
	UserIDKey   = "ginmw.userId"
	TenantIDKey = "ginmw.tenantId"
	RoleKey     = "ginmw.role"
)

type extraClaim struct {
	claim, ctxKey string
}

type jwtConfig struct {
	roleClaim string
	extra     []extraClaim
}

// JWTOption configura JWT.
type JWTOption func(*jwtConfig)

// WithRoleClaim define o nome do claim de perfil/role dentro do token.
// Default: "perfil". Cada projeto usa o nome que já emite nos seus
// tokens -- a lib não força reemitir nada.
func WithRoleClaim(nome string) JWTOption {
	return func(c *jwtConfig) { c.roleClaim = nome }
}

// WithExtraClaim copia um claim string opcional pro contexto do Gin, sob
// a chave ctxKey. Ao contrário de sub/tenantId/perfil, é best-effort: sua
// ausência no token não aborta a requisição (o handler que ler ctxKey via
// ctx.GetString só recebe "" nesse caso). Existe pra claims que um
// projeto carrega no token além dos três exigidos pela lib -- ex: um
// "nome" pra exibir/gravar como autor de uma ação, sem precisar de outra
// consulta ao banco.
func WithExtraClaim(claim, ctxKey string) JWTOption {
	return func(c *jwtConfig) { c.extra = append(c.extra, extraClaim{claim, ctxKey}) }
}

// JWT autentica via cookie "token" ou header "Authorization: Bearer ..."
// (nessa ordem) e exige HMAC + os claims sub, tenantId e o claim de
// perfil configurado. Falta de qualquer um deles aborta com 401 -- não
// existe modo "segue com contexto parcial": um handler que confia em
// TenantID(c) não pode rodar sem ele.
func JWT(secret []byte, opts ...JWTOption) gin.HandlerFunc {
	if len(secret) == 0 {
		panic("ginmw.JWT: secret vazio")
	}

	cfg := jwtConfig{roleClaim: "perfil"}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(c *gin.Context) {
		var tokenString string

		if cookie, err := c.Cookie("token"); err == nil && cookie != "" {
			tokenString = cookie
		} else if header := c.GetHeader("Authorization"); strings.HasPrefix(header, "Bearer ") {
			tokenString = strings.TrimPrefix(header, "Bearer ")
		}

		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Acesso negado, token ausente ou formato invalido"})
			return
		}

		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("método de assinatura inesperado: %v", t.Header["alg"])
			}
			return secret, nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Token inválido ou expirado"})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Falha ao processar permissões"})
			return
		}

		userID, ok := claims["sub"].(float64)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Token não contém identificação do usuário"})
			return
		}

		tenantID, ok := claims["tenantId"].(float64)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Token não contém tenant"})
			return
		}

		role, ok := claims[cfg.roleClaim].(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Token não contém perfil"})
			return
		}

		c.Set(UserIDKey, int64(userID))
		c.Set(TenantIDKey, int64(tenantID))
		c.Set(RoleKey, role)

		for _, e := range cfg.extra {
			if v, ok := claims[e.claim].(string); ok {
				c.Set(e.ctxKey, v)
			}
		}

		c.Next()
	}
}

// UserID devolve o usuario.id do claim `sub`.
func UserID(c *gin.Context) (int64, bool) {
	val, exists := c.Get(UserIDKey)
	if !exists {
		return 0, false
	}
	id, ok := val.(int64)
	return id, ok
}

// TenantID devolve o tenant do JWT -- autoritativo em qualquer rota já
// autenticada. Não confundir com TenantIDFromHeader: usar o header pra
// autorizar escrita pós-login deixaria um admin do tenant A escrever no
// tenant B só trocando X-Tenant-ID.
func TenantID(c *gin.Context) (int64, bool) {
	val, exists := c.Get(TenantIDKey)
	if !exists {
		return 0, false
	}
	id, ok := val.(int64)
	return id, ok
}

// Role devolve o perfil/role do claim configurado via WithRoleClaim.
func Role(c *gin.Context) (string, bool) {
	val, exists := c.Get(RoleKey)
	if !exists {
		return "", false
	}
	role, ok := val.(string)
	return role, ok
}
