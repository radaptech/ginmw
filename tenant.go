package ginmw

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// TenantIDHeaderKey é a chave de contexto que Tenant preenche. Exportada
// pelo mesmo motivo de UserIDKey/TenantIDKey/RoleKey (jwt.go): dar aos
// testes um jeito oficial de simular o middleware, sem forçar todo teste
// a montar um subdomínio + lookup fake.
const TenantIDHeaderKey = "ginmw.tenantIdHeader"

// TenantLookupFunc resolve um subdomínio pro ID do tenant. Cada projeto
// passa a sua consulta (a lib não pode depender do pacote de repository
// de nenhum dos dois) -- ex: queries.ObterEmpresaPorSubdominio.
type TenantLookupFunc func(ctx context.Context, subdominio string) (id int64, err error)

// Tenant lê X-Tenant-ID, rejeita vazio e os subdomínios reservados
// (www, api), resolve via lookup e guarda o ID no contexto. Roda antes
// do login (não depende de token) -- veja TenantIDFromHeader.
//
// notFound é o sentinel que lookup devolve quando não acha nada
// (sql.ErrNoRows, pgx.ErrNoRows, etc.), comparado via errors.Is pra
// decidir 404 vs 500. O erro real nunca vai pro corpo da resposta --
// só pro log do servidor -- porque devolver err.Error() pro cliente
// vaza detalhe interno (mensagem de driver, nome de coluna etc.).
func Tenant(lookup TenantLookupFunc, notFound error) gin.HandlerFunc {
	return func(c *gin.Context) {
		subdominio := strings.TrimSpace(c.GetHeader("X-Tenant-ID"))

		if subdominio == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Tenant não informado (X-Tenant-ID ausente)"})
			return
		}

		if subdominio == "www" || subdominio == "api" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Subdomínio reservado"})
			return
		}

		id, err := lookup(c.Request.Context(), subdominio)
		if err != nil {
			if errors.Is(err, notFound) {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Empresa não encontrada"})
				return
			}
			log.Printf("ginmw: erro ao resolver tenant %q: %v", subdominio, err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Erro interno ao validar empresa"})
			return
		}

		c.Set(TenantIDHeaderKey, id)
		c.Next()
	}
}

// TenantIDFromHeader devolve o tenant resolvido do header X-Tenant-ID.
// É o valor cru do header, sem assinatura -- nunca use pra autorizar
// escrita depois do login; use TenantID (do token) nesse caso.
func TenantIDFromHeader(c *gin.Context) (int64, bool) {
	val, exists := c.Get(TenantIDHeaderKey)
	if !exists {
		return 0, false
	}
	id, ok := val.(int64)
	return id, ok
}
