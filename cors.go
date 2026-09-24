package ginmw

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS libera o domínio de produção (e seus subdomínios) mais localhost
// (e *.localhost) para desenvolvimento, sempre. Domain é passado sem
// protocolo, ex: "radaptech.com.br".
func CORS(domain string) gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOriginFunc: func(origin string) bool {
			origin = strings.TrimSuffix(origin, "/")

			if origin == "http://localhost" || strings.HasPrefix(origin, "http://localhost:") {
				return true
			}

			if strings.HasSuffix(origin, ".localhost") || strings.Contains(origin, ".localhost:") {
				return true
			}

			if origin == "https://"+domain ||
				strings.HasSuffix(origin, "."+domain) ||
				strings.Contains(origin, "."+domain+":") {
				return true
			}

			return false
		},

		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},

		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Content-Length",
			"Accept",
			"Authorization",
			"X-Requested-With",
			"X-Tenant-ID",
		},

		ExposeHeaders: []string{"Content-Length", "Content-Disposition", "Set-Cookie"},

		AllowCredentials:          true,
		OptionsResponseStatusCode: http.StatusNoContent,
		MaxAge:                    12 * time.Hour,
	})
}
