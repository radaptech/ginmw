package ginmw

import "github.com/gin-gonic/gin"

// SecurityHeaders aplica headers básicos de segurança em toda resposta.
// Não inclui HSTS: só faz sentido atrás de HTTPS terminado, e ligar sem
// isso pode travar o próprio localhost.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Content-Security-Policy", "default-src 'self'")

		c.Next()
	}
}
