package ginmw

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Timeout dá prazo a todo request. Sem ele, quem decide quando desistir de
// uma conexão morta com o banco é o kernel (tcp_retries2=15, ~15min) --
// segurou logins por 15min quando o pooler derrubou conexões ociosas sem
// mandar RST. Com prazo, o driver do banco força SetDeadline no socket, a
// query volta em segundos e a conexão zumbi é descartada do pool em vez de
// envenenar os próximos requests.
//
// Funciona porque todo controller já passa c.Request.Context() adiante;
// trocar o contexto do *http.Request aqui propaga pra todas as rotas.
//
// Escolha o prazo com folga: upload multipart dentro de uma transação
// precisa de mais tempo do que uma query simples.
func Timeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Writer = &writerComPrazo{ResponseWriter: c.Writer, ctx: ctx}
		c.Next()
	}
}

// Sem isto, um request que estourou o prazo responde o mesmo 500 genérico
// de um bug de verdade, e em métricas/logs os dois viram a mesma barra. Em
// vez de cada controller checar o próprio prazo, o writer traduz o 500 na
// saída: se o contexto do request expirou, o que aconteceu foi timeout, não
// erro interno.
type writerComPrazo struct {
	gin.ResponseWriter
	ctx        context.Context
	substituir bool
}

func (w *writerComPrazo) WriteHeader(status int) {
	if status == http.StatusInternalServerError && errors.Is(w.ctx.Err(), context.DeadlineExceeded) {
		w.substituir = true
		w.ResponseWriter.WriteHeader(http.StatusGatewayTimeout)
		return
	}
	w.ResponseWriter.WriteHeader(status)
}

// Descarta o corpo do handler e responde o que de fato houve. Devolve
// len(b) porque é o que o render do Gin espera ter escrito.
func (w *writerComPrazo) Write(b []byte) (int, error) {
	if w.substituir {
		w.substituir = false
		w.ResponseWriter.Write([]byte(`{"error":"tempo de resposta esgotado"}`))
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}

func (w *writerComPrazo) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}
