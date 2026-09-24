# ginmw — design

Lib pessoal de middlewares Gin, extraída de dois backends internos em
produção (chamados aqui de Projeto A e Projeto B) que tinham cada um a
sua cópia quase idêntica — e já divergente — desse código. Uso único
(você), sem consumidor externo — o design prioriza corrigir a
divergência entre os dois, não flexibilidade genérica.

## Não-objetivos

- Suportar outro router além do Gin. Os dois projetos usam Gin; abstrair
  isso é resolver um problema que não existe.
- Um `Config` genérico que cobre todo caso futuro. Só vira parâmetro o que
  já muda de um projeto pro outro *hoje*.
- Um README para cada consumidor decidir sozinho. Publicado em
  `github.com/radaptech/ginmw` desde que os dois projetos foram pra
  produção com ele (veja "Adoção" no fim) — atualizado aqui pra registrar
  quando isso mudou, não porque a decisão original estava errada.

## Pacote

Um pacote só, `ginmw`, um arquivo por middleware — espelha o que já existe
em `middleware/`, só que sem duplicar:

```
ginmw/
  cors.go       CORS(domain string) gin.HandlerFunc
  ratelimit.go  RateLimit(rate.Limit, burst int) gin.HandlerFunc
  timeout.go    Timeout(d time.Duration) gin.HandlerFunc
  jwt.go        JWT(secret []byte, opts ...JWTOption) gin.HandlerFunc
  tenant.go     Tenant(lookup TenantLookupFunc) gin.HandlerFunc
  perfil.go     Require(values ...string) gin.HandlerFunc
  headers.go    SecurityHeaders() gin.HandlerFunc
  doc.go        godoc do pacote
```

Sem subpacotes: 7 arquivos não justificam hierarquia.

## Decisões que resolvem as divergências encontradas

Cada item aqui existe porque os dois projetos fazem diferente hoje — a
lista é o motivo da lib, não só um changelog.

### 1. Claim do perfil/role é configurável, não fixo

Projeto A usa `"perfil"`, Projeto B usa `"role"`. Em vez de forçar um
nome (quebraria os tokens já emitidos em produção), a chave do claim é
uma opção:

```go
JWT(secret, WithRoleClaim("perfil"))  // default: "perfil"
```

Migrar os dois projetos pro mesmo claim é decisão de vocês (dos tokens),
não da lib.

### 2. IDs são sempre `int64`

Projeto A usa `int64`, Projeto B usa `int32` (herdado do sqlc). A lib
padroniza em `int64` — é o que `jwt.MapClaims` já entrega (claims JSON
viram `float64`, e a conversão natural é pra `int64`). Nos call-sites do
Projeto B que esperam `int32`, um cast explícito no controller. É custo
de migração do Projeto B, não da lib — documentado no README, não
resolvido por generics.

### 3. Token incompleto sempre falha fechado

Projeto A rejeita token sem `tenantId` ou perfil (`401`); Projeto B
aceita e segue com contexto parcial (claim ausente tratada com `if ok`
sem `else`, no meio do parsing do token). A lib segue o comportamento do
Projeto A: **qualquer claim obrigatório ausente aborta com 401**. Isso
é correção de bug, não uma opção — não dá pra configurar "ignorar claim
ausente".

### 4. Corpo de erro nunca vaza detalhe interno

O middleware de tenant do Projeto B devolvia `"detalhes": err.Error()`
pro cliente. A lib nunca inclui erro cru na resposta — loga server-side
(via um `log.Printf` simples, sem logger injetável, é o que os dois já
fazem) e devolve só `{"error": "<mensagem fixa>"}`.

### 5. Uma única chave de resposta: `"error"`

O middleware de RBAC do Projeto B usava `"erro"`; todo o resto usa
`"error"`. A lib usa `"error"` em todo lugar. Front não precisa tratar
as duas.

### 6. Getters seguem um padrão só: `Get<Coisa>(c) (T, bool)`

Hoje: `GetTenantID`, `GetUserID`, `GetUserPerfil` (OK) convivem com
`ctx.GetString("user_role")` direto no controller (Projeto B, sem
helper). A lib expõe getter pra todo valor que ela injeta — nenhum
controller lê a chave do contexto na mão:

```go
UserID(c)   (int64, bool)
TenantID(c) (int64, bool)  // do JWT, autoritativo pós-login
Role(c)     (string, bool)
```

`Tenant` (middleware de subdomínio, pré-login) expõe sua própria
`TenantIDFromHeader(c) (int64, bool)` — chave diferente de `TenantID`,
porque são fontes diferentes (ver decisão 7).

### 7. Duas fontes de tenant continuam separadas, nomeadas sem ambiguidade

O comentário do JWT middleware do Projeto A já registra o motivo: tenant
do header (`X-Tenant-ID`, pré-login) e tenant do token (pós-login) não
podem ser a mesma chave de contexto, senão um admin do tenant A escreve
no tenant B trocando o header. A lib preserva a separação com nomes que
deixam isso óbvio: `TenantIDFromHeader` vs `TenantID`. O Projeto B hoje
usa só a versão de header mesmo após o login (61 call-sites) — ponto de
atenção na migração, não algo que a lib decide por ele.

### 8. CORS: domínio é parâmetro, `localhost` é sempre liberado

O domínio de produção estava hard-coded em cada projeto. Vira
`CORS("radaptech.com.br")`. Liberar `*.localhost` continua sempre ligado
(é ambiente de dev, não precisa de flag).

### 9. `Require` no lugar de três funções de RBAC redundantes

Três funções fazendo a mesma checagem (perfil atual ∈ lista permitida),
espalhadas entre os dois projetos com nomes diferentes. Uma função
variádica cobre todos os casos, incluindo super-admin:

```go
Require("admin", "tecnico")
Require("super_admin")
```

Falha fechada preservada: sem perfil no contexto, nada casa, nega com
403 (nunca 401 — 401 desloga o usuário no front, e aqui a sessão é
válida, só não tem permissão).

### 10. `RateLimit`, `CORS` (estrutura) e `Timeout` migram como estão

Já são idênticos entre os dois projetos (rate limiter por IP com token
bucket, cleanup a cada 5min) ou só faltam num dos dois (Timeout só no
Projeto A, SecurityHeaders só no Projeto B) — sem conflito pra resolver,
só mover. `Timeout` carrega o comentário original sobre o pooler do
Postgres; é contexto que qualquer projeto futuro vai precisar pra não
"otimizar" o prazo de volta pra zero.

## Tenant: lookup injetado, sem depender de `repository`

A lib não pode importar o `repository` de cada projeto (Projeto B usa
`sql.ErrNoRows`, Projeto A usa `pgx.ErrNoRows`, tipos de retorno
diferentes). O middleware recebe uma função:

```go
type TenantLookupFunc func(ctx context.Context, subdominio string) (id int64, err error)

Tenant(lookup TenantLookupFunc, notFound error)
```

`notFound` é o sentinel que o `lookup` devolve quando não acha — cada
projeto passa o seu (`pgx.ErrNoRows` ou `sql.ErrNoRows`), a lib compara
com `errors.Is` e decide 404 vs 500. Isso também resolve um log de debug
que só um dos dois projetos tinha: a lib loga sempre, sem o log a mais.

## Adoção

Publicado em `github.com/radaptech/ginmw` (público, MIT) assim que os
dois projetos foram para produção com ele — até então, o módulo viveu
local (`module local.test/ginmw`, `.test` porque Go exige um ponto no
primeiro segmento do path) e os dois consumidores apontavam pra ele via
`replace` no próprio `go.mod`. Publicar era questão de tempo, não de
design: build de CI/deploy (Railway, no caso) não tem acesso ao disco de
quem desenvolve, então um `replace` com caminho local nunca ia funcionar
fora da máquina onde a lib foi escrita.

Em produção, cada consumidor usa a versão tagueada de verdade:

```
go get github.com/radaptech/ginmw@v0.1.0
```

sem `replace`, sem `go.work`. `go.work` continua útil só pra
desenvolvimento local (editar a lib e já ver o efeito no consumidor sem
publicar uma tag a cada ajuste) — nunca precisa ser commitado no
consumidor, e nunca foi o que fez funcionar em produção.

## Ordem de extração sugerida

1. `cors.go`, `ratelimit.go` — idênticos, zero decisão de design.
2. `headers.go` — trivial, só mover.
3. `jwt.go` — decisões 1, 2, 3 se aplicam aqui.
4. `tenant.go` — decisão 7 e o lookup injetado.
5. `perfil.go` (`Require`) — decisão 9.
6. `timeout.go` — só mover, com o comentário.

Deixo pra depois porque nenhum dos dois tem hoje: métricas, tracing,
logger estruturado. Adiciona quando um projeto precisar, não antes.
