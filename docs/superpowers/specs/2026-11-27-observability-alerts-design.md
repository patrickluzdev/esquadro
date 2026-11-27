# Métricas, painel e alertas para os apps da plataforma

Data: 2026-11-27 · Pedido por: Patrick · Projeto de exemplo: Promo do Duro

## Por que

O Promo do Duro roda na plataforma com o Grafana (otel-lgtm), e o app já manda traces, logs e métricas por OTLP: HTTP, banco, runtime do Go e jobs do River pelo `otelriver`. Mesmo assim o dono "está cego":

- não há painel pronto;
- só existem três alertas, todos por e-mail: disco, memória e "o app ficou mudo".

O job de contagem do Telegram falhou 9 vezes desde a meia-noite (`telegram: 404`, chave de produção que era um texto de espera) e ninguém soube. O objetivo é **ficar sabendo na hora quando algo quebra**, e ter um lugar para olhar.

## Objetivos

1. Alertas que pegam o que de fato quebra num app, no e-mail **e no Telegram** do dono.
2. Um painel no Grafana já montado para cada app.
3. Chave de integração errada aparecendo no deploy, e não um dia depois.
4. Tudo gerado pelo esquadro para qualquer app da plataforma, sem configurar nada à mão no Grafana.

## Fora do escopo

- Métricas de negócio (cliques, vendas, membros). Ficam nos relatórios do próprio app.
- Tracing distribuído entre serviços, SLOs formais e plantão com escala.
- Outros canais de alerta (Slack, Discord, SMS).

## Anti-objetivos

- Alerta demais: avisos repetidos ou barulhentos, que fazem o dono parar de ler. Cada alerta precisa de um motivo para agir.
- Alertas que dependem de alguém lembrar de configurar o Grafana à mão.
- Segredo (token do robô de alertas) dentro do repositório sem criptografia.

## Abordagens consideradas

| | Como | Troca |
|---|---|---|
| **Escolhida** | O app exporta as métricas que faltam (fila por estado, atraso dos jobs agendados, resultado das checagens). A plataforma provisiona alertas e o painel no Grafana a partir de nomes de métricas fixos, iguais para todo app. | A plataforma não precisa conhecer os jobs de cada app. O app diz o que é "atrasado", porque sabe os próprios horários. |
| A plataforma gera um alerta por job agendado | O `platform.esq` leria o modelo de cada app | Acopla a plataforma ao modelo dos apps, e cada mudança de job exige `task apply`. |
| Consultar o banco direto do Grafana (`river_job`) | Fonte de dados Postgres no Grafana | Dá ao Grafana acesso ao banco dos apps, um risco a mais para um ganho pequeno. |

Canais escolhidos pelo Patrick: **e-mail e Telegram**.

## Desenho

### 1. O que o app passa a exportar (blueprints `jobs`, `integration` e `observability`)

Só quando o app tem o blueprint `observability`:

| Métrica (OTel) | Tipo | Atributos | De onde |
|---|---|---|---|
| `jobs.queue` | gauge | `kind`, `state` (available, running, retryable, scheduled, discarded) | O `worker` conta a `river_job` por tipo e estado a cada minuto. Discarded só das últimas 24 h. |
| `jobs.periodic.overdue` | gauge (0/1) | `kind` | Para cada job com `@jobs(every …)`: 1 quando o último sucesso é mais antigo que o período do agendamento mais uma folga (padrão: o período e meio). Calculado pelo `worker`, que sabe os horários. |
| `jobs.discarded` | counter | `kind` | Somado pelo middleware de jobs quando o River desiste de um job de vez. |
| `integration.check` | gauge (0/1) | `integration` | Resultado do `Check` de cada integração que tem um: na subida do processo e depois a cada hora. |

**Último sucesso dos jobs agendados** *(decisão minha)*: o River apaga os jobs concluídos depois de 24 h, então ele não serve para saber o último sucesso de um job diário. O blueprint `jobs` ganha:
- uma tabela pequena, `jobs_periodic_runs` (`kind`, `last_success_at`, `last_error_at`, `last_error`), com migração própria;
- um middleware que grava nela ao fim de cada execução de um job agendado.

O último erro dessa tabela também entra no texto do alerta. Um job que ainda não rodou desde o deploy conta a partir da subida do worker: só fica atrasado quando passa o primeiro horário previsto mais a folga.

As métricas que já existem continuam:
- `river.work_count`/`river.work_duration` do `otelriver`, com resultado ok/erro;
- as de HTTP do `otelhttp`;
- as do pool do `otelpgx`;
- as de runtime.

**Checagem das integrações na subida** *(decisão minha: só quem pede)*: uma opção nova da integração, `check true` (padrão `false`), liga a checagem. Sem ela, nada é chamado, porque o `Check` padrão (`GET /`) não diz nada na maioria das APIs, e algumas têm cota. Com ela, o `worker` roda o `Check` ao subir e depois a cada hora; o `serve` só roda ao subir. No Promo do Duro, liga para o Telegram. Quando falha:
- registra um log de erro com a integração e o erro (sem o segredo, porque o cliente já esconde a chave do texto do erro);
- grava `integration.check = 0`.

O processo **não para**: uma integração fora do ar não pode derrubar o app inteiro.

**Higiene de chave** (no blueprint `integration`, para integrações com `Key`): tirar espaços e quebras de linha das pontas ao carregar. Para o Telegram em particular, aceitar o token colado com o prefixo `bot` sem duplicar. Isso fica no template da integração `telegram`, se ela for um modelo do esquadro; senão, no código do app.

### 2. Alertas (blueprint `platform/observability`)

Provisionados em `alerts.yaml.j2`, um conjunto por app. O "app ficou mudo" que já existe continua. Os limites são opções do blueprint, com os padrões abaixo:

| Alerta | Regra (PromQL; os nomes exatos depois da conversão OTLP → Prometheus) | Dispara depois de | Padrão |
|---|---|---|---|
| Job falhando | `increase(river_work_count{status="error"}[15m]) > 0`, por `job` e `kind` | 0 min (avisa na primeira falha) e repete no máximo a cada 4 h | ligado |
| Job agendado atrasado | `jobs_periodic_overdue == 1` | 10 min | ligado |
| Jobs presos tentando de novo | `jobs_queue{state="retryable"} > 0` | 1 h | ligado |
| Jobs descartados | `increase(jobs_discarded_total[15m]) > 0` (desistiu de vez) | 0 min | ligado |
| Integração com erro | `integration_check == 0` | 5 min | ligado |
| API com erro | razão de respostas 5xx no total > 5% com pelo menos 20 requisições em 10 min | 10 min | 5% |
| API lenta | p95 de `http.server.request.duration` > 2 s | 15 min | 2 s |
| App reiniciando | o processo reiniciou mais de 3 vezes em 30 min (pelo início do processo nas métricas de runtime ou no `target_info`) | 0 min | 3 |
| Backup não rodou | o último backup com sucesso tem mais de 26 h (ver abaixo) | 0 min | 26 h |

**Língua e limites** *(decisão minha)*: valem para a plataforma inteira, com opções do blueprint `platform/observability` (`alerts_language`, padrão `en`, e um limite por alerta). A plataforma tem um dono só, e os dois apps de hoje são dele. Um limite por app, se um dia for preciso, vira um campo do bloco `app` do `platform.esq`.

**Alertas que já existem** *(decisão minha)*: disco, memória e "o app ficou mudo" continuam como estão, com a mesma política de e-mail e a repetição a cada 12 h. Só ganham o Telegram como segundo canal. Os alertas novos usam a política nova (agrupados por app e alerta, com a repetição da tabela).

O texto de cada alerta diz:
- o app;
- o que quebrou (por exemplo, "o job deals.count_telegram_members falhou: telegram: 404");
- onde olhar: o link para o painel do Grafana e, quando a tela de jobs estiver ligada, o link para ela. *(Decisão minha:)* o app manda o endereço da tela como atributo do recurso OTel (`app.jobs_ui.url`), e a plataforma não precisa ler o modelo do app. Por isso o desenho da tela de jobs (`2026-11-27-jobs-ui-design.md`) é construído **primeiro**.

Junto do alerta vai o último erro do job: buscado nos logs (Loki), ou mandado como atributo da métrica de falha só com o tipo de erro, se buscar nos logs não der.

Também avisa quando resolveu ("voltou ao normal").

**Backup:** o comando de backup da plataforma, ao terminar com sucesso, manda uma métrica por OTLP para o `otel-lgtm` (`platform.backup.last_success`, com o atributo `app`, em segundos desde 1970). O backup roda no servidor, fora da rede do Docker, então a porta OTLP do `otel-lgtm` (4318) passa a ser publicada **só em 127.0.0.1**. Como a métrica chega uma vez por dia, a regra usa `last_over_time(…[27h])` e compara com o horário atual.

### 3. Canais (blueprint `platform/observability`)

- **E-mail:** o que já existe (`platform-owner-email`).
- **Telegram:** um ponto de contato novo do Grafana, do tipo `telegram`. Usa dois segredos da plataforma, em `secrets/platform.sops.yaml`: `alerts_telegram_token` e `alerts_telegram_chat_id`.
  - O `task secrets:init` cria os dois vazios.
  - Vazios, o canal não é criado e só o e-mail vale. O `task apply` avisa que o Telegram ficou desligado.
- A política de envio manda todo alerta aos dois canais, agrupado por app e alerta. A espera e o intervalo de repetição seguem a tabela acima.
- A doc do blueprint explica como pegar os dois valores. O `chat_id` sai do `getUpdates` do robô depois de mandar `/start` para ele. Recomenda-se um robô só de alertas, separado do robô do app, para um problema num não calar o outro.

### 4. Painel (blueprint `platform/observability`)

Um painel do Grafana provisionado por app (`dashboards/<app>.json`, gerado pelo Ansible), na pasta "Apps":

1. requisições por minuto, erros 5xx e p50/p95 por rota;
2. jobs: execuções por tipo e resultado, duração, fila por estado e agendados atrasados;
3. integrações: a última checagem de cada uma;
4. banco: conexões do pool, em uso e esperando;
5. processo: memória, goroutines e reinícios;
6. últimos logs com erro do app (Loki).

### 5. Promo do Duro

- Regenerar com o esquadro novo, depois `task apply` na plataforma e um deploy do app (`api/v*`). As duas coisas só com o OK do Patrick.
- O Patrick cria o robô de alertas, manda `/start` para ele e põe os dois segredos com o sops. Eu não vejo nem guardo esses valores.
- Para testar ponta a ponta: forçar um alerta de teste pelo Grafana (o "Test" do ponto de contato) e confirmar que chegou no e-mail e no Telegram.

## Como ficou na construção

- **Nomes confirmados num Grafana local (otel-lgtm 0.39):**
  - `river_work_count_total` (com `kind`, `status`, `attempt`);
  - `jobs_queue`, `jobs_periodic_overdue`, `jobs_discarded_total`, `integration_check`;
  - `http_server_request_duration_seconds_*` (com `http_route`, `http_response_status_code`);
  - `pgxpool_*`, `go_memory_used_bytes`, `target_info`.
- **API e worker se sobrescreviam:** os dois processos de um app mandavam métricas com os mesmos rótulos. Cada processo agora manda o próprio `service.instance.id`, e isso também é o que detecta reinícios (processos distintos em 30 min acima de `processos × (1 + alerts_restarts)`).
- **A checagem de integração roda só no worker**, ao subir e a cada hora, e não no `serve`. Um processo só basta para o alerta, e assim cada API externa recebe metade das chamadas.
- **O link para a tela de jobs não entrou no texto do alerta.** O texto aponta para o painel do app no Grafana e diz que o erro está na tela de jobs. Isso evita um atributo novo entre o app e a plataforma.
- **O prefixo "bot" na chave do Telegram não é tratado no esquadro.** O jeito de autenticar fica no arquivo da integração, que é do app. O esquadro só tira espaços e quebras de linha das pontas da chave.
- **O token do robô de alertas não aparece no arquivo de alertas:** ele fica no `.env` do Grafana (só root lê), e o arquivo usa `${ALERTS_TELEGRAM_TOKEN}`. Testado: o Grafana substitui pelo valor do ambiente.
- **Testado ponta a ponta num app gerado:** um job de e-mail que falha disparou "apiui: um job falhou mail_send" no Grafana local, com o painel carregado.

## Restrições

- Esquadro só genérico; os nomes do Promo do Duro ficam só nos exemplos desta doc.
- Código em inglês, textos dos alertas em pt-BR (o dono fala pt-BR; a língua dos alertas é uma opção do blueprint, padrão `en`, e o Promo do Duro usa `pt-BR`).
- Nada de segredo em texto puro no repositório.
- Nada em produção (apply, deploy) sem OK explícito.

## Fica para quem construir

- Os nomes exatos das métricas no Prometheus depois da conversão OTLP (sufixos `_total`, `_seconds`, `_bucket`). Conferir no `otel-lgtm` de desenvolvimento antes de escrever as regras.
- Como detectar reinício com o que já é exportado, ou se é preciso uma métrica de início do processo.
- Se o último erro do job vai no texto do alerta por Loki ou por atributo.
- O desenho visual do painel (ordem e tamanho dos quadros), dentro das seis seções acima.
- Os testes:
  - os templates geram YAML/JSON válidos;
  - as métricas novas aparecem num app de teste;
  - um job que falha dispara o alerta no Grafana de desenvolvimento.
