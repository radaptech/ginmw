# ginmw

Middlewares Gin para SaaS multi-tenant: CORS, rate limit, timeout, headers
de segurança, JWT, tenant por subdomínio e checagem de perfil — extraídos
de dois backends em produção que tinham, cada um, a sua cópia quase
idêntica (e já divergente) desse código.

Uso pessoal, sem garantia de API estável entre versões — mas dá pra usar.
As decisões de design (por que cada coisa é do jeito que é) estão em
[DESIGN.md](DESIGN.md).

## Por que existe

Dois projetos, mesma stack (Gin + JWT + tenant por subdomínio), mesmo
código de middleware copiado e colado — e já divergente o suficiente pra
um ter corrigido um bug que o outro ainda carregava. Esta lib existe pra
essa correção se propagar uma vez só, em vez de duas.

## Instalação

```
go get github.com/radaptech/ginmw
```

```go
import "github.com/radaptech/ginmw"
```

## Uso

```go
import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"golang.org/x/time/rate"
	"github.com/radaptech/ginmw"
)

func main() {
	r := gin.New()

	r.Use(
		ginmw.CORS("radaptech.com.br"),
		ginmw.SecurityHeaders(),
		ginmw.Timeout(30*time.Second),
	)

	login := r.Group("/autenticacao")
	login.Use(ginmw.RateLimit(rate.Every(12*time.Second), 5))

	tenant := ginmw.Tenant(buscarTenantPorSubdominio, pgx.ErrNoRows)
	auth := ginmw.JWT([]byte(secret), ginmw.WithRoleClaim("perfil"))

	api := r.Group("/api", tenant, auth)
	api.GET("/maquinas", ginmw.Require("admin", "tecnico"), listarMaquinas)

	r.Run()
}

func buscarTenantPorSubdominio(ctx context.Context, sub string) (int64, error) {
	empresa, err := queries.ObterEmpresaPorSubdominio(ctx, sub)
	return empresa.ID, err
}

func listarMaquinas(c *gin.Context) {
	tenantID, _ := ginmw.TenantID(c)
	userID, _ := ginmw.UserID(c)
	role, _ := ginmw.Role(c)
	// ...
}
```

## Referência

### `CORS(domain string) gin.HandlerFunc`

Libera `https://<domain>` e subdomínios, mais `localhost`/`*.localhost`
sempre (dev). Credentials habilitado, preflight cacheado por 12h.

### `RateLimit(rate rate.Limit, burst int) gin.HandlerFunc`

Token bucket por IP (`c.ClientIP()`). Entradas sem uso há 10min são
limpas a cada 5min. Para rotas sensíveis a força bruta (login, reset de
senha) — não é rate limit global da API.

### `Timeout(d time.Duration) gin.HandlerFunc`

Aplica `d` ao contexto do request. Se o handler responder 500 com o
contexto já expirado, a resposta vira 504 com
`{"error": "tempo de resposta esgotado"}` — evita confundir timeout com
bug de verdade nos logs/métricas.

### `SecurityHeaders() gin.HandlerFunc`

`X-Content-Type-Options`, `X-Frame-Options: DENY`,
`Content-Security-Policy: default-src 'self'`. Sem HSTS (só faz sentido
atrás de HTTPS terminado).

### `JWT(secret []byte, opts ...JWTOption) gin.HandlerFunc`

Lê o token do cookie `token` ou do header `Authorization: Bearer <token>`
(nessa ordem). Exige HMAC e os claims `sub`, `tenantId` e o claim de
perfil (nome configurável) — **falta de qualquer um deles aborta com
401**, nunca segue com contexto parcial.

Opções:

- `WithRoleClaim(nome string)` — nome do claim de perfil no token.
  Default: `"perfil"`.
- `WithExtraClaim(claim, ctxKey string)` — copia um claim string opcional
  pro contexto sob `ctxKey`. Best-effort: ausente no token, não aborta a
  requisição, só não seta a chave. Empilha (dá pra chamar mais de uma
  vez). Ex: `WithExtraClaim("nome", "user_nome")` pra ler depois com
  `ctx.GetString("user_nome")`.

Getters (usam `401` implícito: se o middleware não rodou, `ok` é
`false`):

```go
ginmw.UserID(c)   (int64, bool)
ginmw.TenantID(c) (int64, bool)  // do token — autoritativo pós-login
ginmw.Role(c)     (string, bool)
```

Em produção, sempre pelos getters acima. `ginmw.UserIDKey`, `ginmw.TenantIDKey`
e `ginmw.RoleKey` são exportadas só pra simular um contexto autenticado em
teste, sem passar por um token de verdade:

```go
ctx.Set(ginmw.UserIDKey, int64(1))
ctx.Set(ginmw.TenantIDKey, int64(7))
ctx.Set(ginmw.RoleKey, "administrador")
```

### `Tenant(lookup TenantLookupFunc, notFound error) gin.HandlerFunc`

Lê `X-Tenant-ID` do header, rejeita `www`/`api` (subdomínios
reservados), resolve via `lookup` e guarda o ID no contexto. `notFound`
é o sentinel de "não achou" que o seu `lookup` devolve (`sql.ErrNoRows`,
`pgx.ErrNoRows`, etc.) — comparado com `errors.Is`.

Roda **antes** do login (não depende de token), por isso a chave que
expõe é separada da do JWT:

```go
ginmw.TenantIDFromHeader(c) (int64, bool)
```

Nunca use `TenantIDFromHeader` pra autorizar escrita depois do login —
é o header cru, qualquer cliente troca. Pós-login, autoridade é
`ginmw.TenantID` (vem do token assinado). `ginmw.TenantIDHeaderKey` existe
pelo mesmo motivo de `UserIDKey`/`TenantIDKey`/`RoleKey`: só pra teste.

Em erro, a resposta nunca inclui `err.Error()` — só
`{"error": "<mensagem fixa>"}`; o erro real vai pro log do servidor.

### `Require(perfis ...string) gin.HandlerFunc`

Roda depois de `JWT`. Perfil atual fora da lista → `403` (nunca `401`:
sessão é válida, só falta permissão). Sem perfil no contexto (JWT não
rodou antes) → nega, falha fechada.

## Não faz (de propósito)

- Não suporta outro router além do Gin.
- Não tem logger nem métricas — cada projeto já tem o seu.
- Não decide o nome dos claims do seu token (exceto os 3 obrigatórios) —
  configura via `WithRoleClaim`/`WithExtraClaim`, sem forçar reemitir
  tokens já em produção.

Motivo de cada não-objetivo em [DESIGN.md](DESIGN.md).

## Desenvolvendo localmente (contra um consumidor)

Pra testar uma mudança nesta lib direto num projeto que a usa, sem
publicar uma tag a cada ajuste, use um `go.work` na raiz que contém os
dois:

```
go work init ./ginmw ./seu-projeto
```

O `go build`/`go test` do seu projeto passam a enxergar o código local
do `ginmw` automaticamente. `go.work` fica só na sua máquina — não
precisa (nem deve) ser commitado no projeto consumidor.

## Licença

MIT — veja [LICENSE](LICENSE).
