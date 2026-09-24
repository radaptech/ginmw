package ginmw

import (
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
)

// Require barra a rota pra quem não tem um dos perfis listados. Roda
// sempre depois de JWT, que é quem põe o Role no contexto.
//
// 403, nunca 401: 401 é "sem sessão" e o front desloga o usuário em
// qualquer 401 fora do login -- alguém sem permissão pra uma rota não
// devia ser expulso do sistema, só barrado ali.
//
// Falha fechada: sem role no contexto (JWT não rodou antes) nenhum
// perfil casa e a rota nega.
func Require(perfis ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := Role(c)

		if !slices.Contains(perfis, role) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "Acesso negado: privilégios insuficientes",
			})
			return
		}

		c.Next()
	}
}
