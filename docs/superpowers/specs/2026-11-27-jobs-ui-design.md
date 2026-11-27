# Tela da fila de jobs (River UI) atrás do login

Data: 2026-11-27 · Pedido por: Patrick · Projeto de exemplo: Promo do Duro

## Por que

O job do Promo do Duro que conta os membros do Telegram falhou 9 vezes seguidas (`404 Not Found`, chave do robô inválida em produção) e ninguém viu. Para descobrir, foi preciso entrar no servidor por SSH e consultar a tabela `river_job` à mão. O dono do app quer ver a fila (o que rodou, o que falhou, com que erro, o que está tentando de novo) e agir nela (tentar de novo, cancelar) pelo navegador, só ele ou quem tiver o papel certo, sem SSH.

## Objetivos

- Uma opção do blueprint `jobs` que serve a tela oficial do River (`riverui`) num caminho do app.
- Acesso só para quem está logado no app com um dos papéis configurados (padrão: os administradores).
- Desligada por padrão. Ligar é uma linha no `.esq`.

## Fora do escopo

- Uma tela própria de jobs no painel (`admin`). A tela do River já faz isso.
- Alertas de job. Ficam no desenho de observabilidade (`2026-11-27-observability-alerts-design.md`).
- Acesso por chave de serviço (`Authorization: Bearer sk_…`). A tela é para pessoas.

## Anti-objetivos (seria falha mesmo funcionando)

- A tela abrir para quem não está logado, ou para um gestor quando só o administrador pode.
- Afrouxar o CSP ou o CORS do resto da API para a tela funcionar.
- Exigir uma senha, um domínio ou um serviço a mais na plataforma.

## Abordagens consideradas

| | Como | Troca |
|---|---|---|
| **A (escolhida)** | `riverui` montado dentro do app, num caminho (`/jobs`), atrás da sessão e dos papéis do blueprint `security` | Mesmo login e mesmas regras do resto. O binário fica alguns MB maior (o frontend do riverui vai embutido). |
| B | Serviço `riverui` à parte na plataforma, só por túnel SSH (`task river`) | É o mais fechado, mas precisa de terminal toda vez e não abre no celular. |
| C | Serviço à parte com domínio próprio e senha no Caddy | É uma senha a mais, sem ganho sobre a A. |

O Patrick escolheu a A.

## Desenho

### No `.esq`

```
blueprint jobs {
  ui        "/jobs"            // caminho da tela; vazio = desligada (padrão)
  ui_access [user.admin]       // regras realm.role, como em @security; padrão: [user.admin]
}
```

- `ui` é uma opção nova do blueprint `jobs` (string, padrão `""`).
- `ui_access` é uma lista de regras no mesmo formato das de `@security(read [...])`. Padrão: os administradores do primeiro realm, do mesmo jeito que o `service_admins` do `security` já resolve "quem administra".
- `ui` sem o blueprint `security` (sem principal) é um erro de validação do esquadro: a tela não pode existir sem login. A mensagem diz para adicionar `security` ou tirar `ui`.

### No app gerado

- O processo `serve` (o da API) monta o `riverui` em `<ui>` pelo slot `http.routes`. O handler usa um cliente River só de inserção. O `serve` não trabalha a fila, e essa continua sendo tarefa do `worker`.
- Antes do handler, um middleware de acesso:
  - **sem sessão:** responde uma página curta em pt-BR/en (pelo `i18n`, quando houver) dizendo para entrar no painel e voltar a este endereço. Não redireciona sozinho para outro domínio.
  - **com sessão, mas sem um dos papéis de `ui_access`:** 403 com a mesma página, dizendo que falta permissão.
  - **com o papel certo:** passa. Vale também para a API interna do riverui (`<ui>/api/...`), que precisa passar pelo mesmo middleware.
- A sessão é a do cookie que o `security` já usa (`HttpOnly`, `SameSite=Lax`, no domínio da API). Abrir `https://api.<app>/jobs` direto no navegador leva o cookie.
- Ações que mudam a fila (tentar de novo, cancelar, apagar) vêm como POST da própria tela. O frontend do `riverui` não manda o `X-CSRF-Token` do app. Por isso, *(decisão minha)*:
  - a tela **não** passa pela corrente autenticada comum (`security.Authenticate`, que exige o token de CSRF), e o CSRF do resto da API não muda;
  - o blueprint `security` passa a exportar um invólucro genérico para "interface de terceiros embutida" (por exemplo `security.EmbeddedUI(rules, handler)`), que lê a sessão do cookie e confere os papéis do mesmo jeito que `Authenticate`/`Allows`;
  - no lugar do token, protege os métodos que não são GET exigindo `Origin` igual ao endereço da API (sem `Origin`, exige `Referer` da mesma origem; sem os dois, recusa), além do cookie `SameSite=Lax`;
  - o blueprint `jobs` usa esse invólucro em `<ui>`, e o mesmo invólucro serve depois para outras telas embutidas.
- **Cabeçalhos:** o resto da API continua com `default-src 'none'`. Só as respostas sob `<ui>` recebem um CSP próprio que deixa carregar os arquivos da própria origem (`default-src 'self'`, e o que mais o riverui precisar, como `style-src 'self' 'unsafe-inline'` se ele exigir), sempre com `frame-ancestors 'none'`.
- **Registro das ações** *(decisão minha)*: cada requisição que não é GET sob `<ui>` gera uma linha de log estruturada, com quem fez (id e papel), o método e o caminho. Ela vai junto com o trace. A tabela do blueprint `audit` fica de fora nesta versão: ela é feita para entidades com id uuid e ações de CRUD, e saber "qual job" exigiria ler o corpo das requisições da API interna do riverui, que muda a cada versão. Se um dia for preciso auditoria de verdade, isso vira um desenho à parte.
- **Observabilidade:** as rotas sob `<ui>` entram nos traces como as outras, com o nome da rota.

### Na documentação

- O README gerado do app ganha uma linha: onde fica a tela e quem entra.
- A documentação do blueprint `jobs` no site do esquadro explica as duas opções.

### Promo do Duro

- `deals.esq`/`app.esq`: `jobs { ui "/jobs" ui_access [user.admin] }`.
- Endereço: `https://api.promododuro.com.br/jobs`, só para Administrador. O papel Gestor não entra.

## Como ficou na construção

- O `security.Authenticate` continua na frente de todas as rotas, a tela inclusive. `EmbeddedUI(prefix, rules, realms, next)` registra o caminho da tela. Para um caminho registrado, o `Authenticate` não pede o `X-CSRF-Token`, mas exige `OwnOrigin`: `Sec-Fetch-Site: same-origin` ou, em navegador antigo, `Origin`/`Referer` do próprio endereço da API. Nem os frontends que o CORS aceita passam. O resto da API não muda.
- O CSP da tela libera só o script embutido da página do riverui (o que escolhe o tema), pelo hash calculado quando o app sobe. Não usa `'unsafe-inline'` em scripts.
- O River subiu para v0.51.2, junto com o riverui v0.23.0. Não há migração nova de banco do River entre as duas versões (as duas param na 008).
- O README do app é escrito uma vez só (`.once`), então não ganhou a linha. A opção está na página de referência do blueprint `jobs`.
- Testado num app gerado (starter `api`) com Postgres:
  - visitante recebe 401;
  - gestor recebe 403;
  - administrador recebe 200 e a tela abre no navegador, sem erro de CSP;
  - um POST de outro site recebe 403;
  - o resto da API continua exigindo o CSRF.

## Restrições

- Código em inglês. Textos visíveis em pt-BR (e nas outras línguas do `i18n`).
- Esquadro só genérico: nada de "Promo do Duro" no blueprint.
- Usar a versão do `riverui` compatível com o River do blueprint (`v0.50.0`). Se não houver uma compatível, subir os dois juntos, com teste.
- Deploy do Promo do Duro só com o OK explícito do Patrick (tag `api/v*`).

## Fica para quem construir

- O nome exato das opções (`ui`/`ui_access`), se o padrão do esquadro pedir outro.
- O CSP mínimo que o riverui aceita: testar no navegador e não abrir mais do que ele precisa.
- O texto da página de "entre no painel" e de "sem permissão".
- Os testes:
  - sem sessão → página;
  - gestor → 403;
  - administrador → 200;
  - POST de outra origem → 403;
  - `ui` sem `security` → erro de validação.
