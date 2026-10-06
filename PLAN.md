# Plano: Transcrição local de vídeos em Go

Oct 5, 2026

## Visão geral e pré-requisitos

O objetivo é um utilitário em Go, 100% local, que varre uma pasta de vídeos já gravados, extrai o áudio de cada um e gera a transcrição (texto e SRT) usando o whisper.cpp como motor, sem nenhuma dependência de API em nuvem.

Pré-requisitos para instalar uma vez, fora do projeto Go:

- Go 1.22 ou mais recente
- ffmpeg no PATH
- cmake e um compilador C/C++ (gcc ou clang), para compilar o whisper.cpp
- git

Convenções a repetir em todos os prompts das fases seguintes, pra manter o código consistente com o resto do seu trabalho:

- comentários no código em inglês
- logs em Go com `log.Printf`/`fmt.Printf`, sem handler customizado de `slog`
- preferir soluções reutilizáveis (pacotes/funções genéricas em vez de código duplicado por fase)
- nomes de pacotes e identificadores em inglês

## Fluxo de branches

O repositório trabalha com duas branches fixas: `main` e `develop`. Todo desenvolvimento novo (features, correções etc.) parte de `develop`, nunca direto de `main`.

- `develop`: branch de integração, recebe os PRs de feature/fix do dia a dia.
- `main`: branch estável; só recebe merge de `develop` quando o conteúdo acumulado está pronto pra virar uma nova versão.
- É o merge de `develop` pra `main` que dispara o pipeline de release da Fase 7 (geração da tag e build dos binários).

Estrutura de pastas sugerida:

```
scribin/
  cmd/scribin/               # CLI entrypoint (fase 4)
  internal/extract/         # wrapper do ffmpeg (fase 2)
  internal/transcribe/      # wrapper do whisper-cli (fase 3)
  internal/batch/           # orquestração/worker pool (fase 4)
  internal/store/           # persistência opcional no Postgres (fase 5)
  third_party/whisper.cpp/  # clone compilado localmente (fase 1)
  models/                   # modelos ggml baixados
```

Cada fase abaixo tem um prompt pronto para copiar e colar no Claude Code (ou outro agente) direto no diretório do projeto, na ordem em que aparecem.

## Fase 1: Ambiente e dependências

Antes de escrever código Go, vale compilar o whisper.cpp e testar a transcrição isolada, pra garantir que o ambiente local funciona antes de integrar.

```
Estou criando um projeto Go chamado scribin para transcrever vídeos já gravados localmente, usando o whisper.cpp como motor de transcrição (sem nenhuma API em nuvem).

Crie um script `scripts/setup.sh` (bash) que:
1. Clona o repositório https://github.com/ggerganov/whisper.cpp dentro de `third_party/whisper.cpp` (ou atualiza se já existir).
2. Compila com `cmake -B build && cmake --build build -j --config Release`, gerando o binário em `third_party/whisper.cpp/build/bin/whisper-cli`.
3. Baixa o modelo ggml `small` via `models/download-ggml-model.sh small`, salvando em `models/`.
4. Verifica se o `ffmpeg` está instalado (`ffmpeg -version`); se não estiver, imprime instruções de instalação para Linux/macOS e aborta.
5. Ao final, roda uma transcrição de teste com um sample.wav incluído no whisper.cpp (ex: samples/jfk.wav) usando o whisper-cli e imprime o resultado, pra confirmar que a cadeia toda funciona.

O script deve ser idempotente (pode rodar de novo sem duplicar o clone) e dar mensagens claras de erro se qualquer etapa falhar.
```

## Fase 2: Extração de áudio em Go

Com o ambiente validado, o primeiro módulo Go é o que chama o ffmpeg pra tirar o áudio do vídeo e normalizar pro formato que o whisper.cpp espera (WAV 16kHz mono).

```
No projeto Go scribin (módulo já iniciado com `go mod init`), crie o pacote `internal/extract` com uma função:

    func ExtractAudio(ctx context.Context, videoPath, outputDir string) (string, error)


Que:
- Chama o `ffmpeg` via `os/exec` (com `exec.CommandContext` para respeitar cancelamento/timeout) para extrair o áudio do vídeo em `videoPath` e convertê-lo para WAV PCM 16-bit, 16kHz, mono.
- Salva o WAV em `outputDir` com o mesmo nome base do vídeo (ex: `video1.mp4` -> `video1.wav`), e retorna o caminho do WAV gerado.
- Retorna erro claro se o ffmpeg não existir no PATH, se o vídeo não existir, ou se o comando falhar (incluindo stderr do ffmpeg na mensagem de erro).
- Usa `log.Printf` para logar início/fim da extração e o comando executado (sem configurar handler de slog customizado).

Siga convenções: comentários em inglês, nomes de pacote/função em inglês, código reutilizável (sem hardcode de flags que deveriam ser parâmetros). Adicione um teste em `internal/extract/extract_test.go` que valida o tratamento de erro quando o vídeo não existe (não precisa rodar ffmpeg de verdade nesse teste).
```

## Fase 3: Transcrição via whisper-cli

Agora o pacote que chama o binário do whisper.cpp sobre o WAV gerado na fase anterior e devolve o texto transcrito de forma estruturada.

```
No projeto Go scribin, crie o pacote `internal/transcribe` com:

    type Segment struct {
        Start, End float64 // seconds
        Text       string
    }

    type Result struct {
        Segments []Segment
        FullText string
    }

    func Transcribe(ctx context.Context, wavPath string, opts Options) (Result, error)

    type Options struct {
        WhisperBinPath string // path to whisper-cli
        ModelPath      string // path to the .bin ggml model
        Language       string // e.g. "pt", "en", or "" for auto-detect
    }

Implementação:
- Chama o binário `whisper-cli` via `exec.CommandContext` passando `-m <ModelPath> -f <wavPath> -l <Language> -oj` (saída em JSON) e um diretório/arquivo de saída temporário.
- Faz parse do JSON gerado pelo whisper.cpp (formato com array de segments, cada um com `start`, `end`, `text`) pro struct `Result` acima.
- Em caso de erro no binário, retorna o stderr na mensagem de erro.
- Usa `log.Printf` pra logar o comando executado e quanto tempo levou a transcrição.

Siga as mesmas convenções das fases anteriores (comentários em inglês, código reutilizável, sem handler de slog customizado). Adicione um teste que valida o parser do JSON do whisper.cpp usando um JSON de exemplo fixo (sem precisar rodar o binário real).
```

## Fase 4: Orquestração em lote

Com extração e transcrição isoladas, a camada de orquestração varre um diretório/arquivo de vídeos e processa tudo em lote, sem precisar de fila externa.

```
No projeto Go scribin, crie o pacote `internal/batch` e o entrypoint `cmd/scribin/main.go` que:

- Recebem via flags: `--input-dir` (pasta com os vídeos), `--output-dir` (onde salvar as transcrições), `--model` (caminho do modelo ggml), `--whisper-bin` (caminho do whisper-cli), `--lang` (idioma, default "pt"), `--workers` (número de processamentos em paralelo, defalt = metade das CPUs disponíveis).
- Varrem recursivamente `--input-dir` buscando arquivos de vídeo (extensões mp4, mkv, mov, avi).
- Para cada vídeo, pulam o processamento se já existir um `.txt` correspondente em `--output-dir` (pro não reprocessar em reruns).
- Processam os videos pendentes usando um worker pool limitado a `--workers` goroutines simultâneas (via semáforo em channel), chamando `internal/extract.ExtractAudio` e depois `internal/transcribe.Transcribe` pra cada um.
- Escrevem o resultado em `<output-dir>/<nome-do-video>.txt` (texto corrido) e `<nome-do-video>.srt` (com timestamps a partir dos Segments).
- Logam com `log.Printf` o progresso (ex: "[3/20] video1.mp4 transcrito em 42s"), e seguem processando os outros vídeos mesmo se um falhar, reportando um resumo final de sucessos/falhas.

Convenções: comentários em inglês, reutilizar os pacotes `extract` e `transcribe` sem duplicar lógica, sem handler de slog customizado.
```

## Fase 5 (opcional): Persistência no Postgres

Se quiser buscar depois nas transcrições (por palavra-chave, por data, por vídeo), vale indexar o resultado num Postgres local em vez de só deixar os .txt/.srt no disco.

```
No projeto Go scribin, adicione o pacote `internal/store` que:

- Usa `database/sql` com o driver `pgx` (github.com/jackc/pgx/v5/stdlib) pra conectar num Postgres local via DSN vindo de variável de ambiente (`DATABASE_URL`).
- Cria uma migration simples (SQL puro, em `migrations/0001_init.sql`) com uma tabela `transcriptions` (id serial primary key, video_path text not null, model text not null, language text, full_text text not null, created_at timestamptz not null default now()) e uma tabela `segments` (transcription_id fk, start_seconds, end_seconds, text), pra permitir busca por trecho com timestamp.
- Expõe uma função `SaveTranscription(ctx context.Context, db *sql.DB, videoPath, model, language string, result transcribe.Result) error` que grava a transcrição e os segments numa transação só.
- Integre essa chamada no `cmd/scribin/main.go` da fase 4: se `DATABASE_URL` estiver definida, grava no Postgres além de escrever os arquivos .txt/.srt; se não estiver definida, só escreve os arquivos (Postgres fica opcional).

Convenções: comentários em inglês, log com `log.Printf`, tratar erro de conexão com Postgres sem derrubar o processamento dos videos (loga o erro e continua salvando só em arquivo).
```

## Fase 6: Testes, logs e hardening

Última fase: fechar as pontas soltas antes de considerar o pipeline pronto pra uso do dia a dia.

```
No projeto Go scribin, revise o pipeline completo (extract, transcribe, batch, e store se tiver sido implementado) e:

- Adicione testes de integração leves em `cmd/scribin` usando um vídeo de teste bem curto (poucos segundos) incluído em `testdata/`, validando o fluxo ponta a ponta sem exigir GPU (modelo `tiny` ou `base` só pra teste).
- Garanta que cada vídeo processado tem um timeout individual via `context.WithTimeout` (configurável por flag, ex: `--per-video-timeout`), pra um vídeo corrompido não travar o lote inteiro.
- Garanta que erros de um vídeo específico não derrubam o processo: logue com `log.Printf` e siga pro próximo, mas saia com código de saída não-zero no final se houve qualquer falha.
- Adicione um `--dry-run` que só lista quais vídeos seriam processados (útil antes de rodar em lotes grandes), sem chamar ffmpeg/whisper.
- Revise se todos os comentários estão em inglês e se não há nenhum handler customizado de slog (só `log.Printf`/`fmt.Printf`).
- Gere um `README.md` simples explicando como instalar os pré-requisitos (fase 1), compilar, e rodar o comando `scribin` com as flags disponíveis.
```

## Fase 7: Pipeline de release automático (tag no merge pra main)

Com o CLI funcionando, falta automatizar a geração de versão: a cada merge de `develop` pra `main` no GitHub, o pipeline analisa os commits (Conventional Commits) e cria a tag semver automaticamente, publicando binários na release.

```
No repositório GitHub do projeto scribin, crie um workflow do GitHub Actions em `.github/workflows/release.yml` que:

- Dispara em `push` para a branch `main` (ou seja, a cada merge de PR pra main), com permissão `contents: write`.
- Usa a action `mathieudutour/github-tag-action@v6` para analisar as mensagens de commit desde a última tag seguindo Conventional Commits (`feat:` -> minor, `fix:` -> patch, `feat!:`/`fix!:`/"BREAKING CHANGE" no corpo -> major) e criar e empurrar automaticamente a próxima tag semver (ex: v1.2.3), usando o `GITHUB_TOKEN` padrão do workflow. Se nenhum commit relevante for encontrado, não cria tag nova.
- Em seguida, usa a action `goreleaser/goreleaser-action@v7` para compilar o binário de `cmd/scribin` para Linux, macOS e Windows (amd64 e arm64) e publicar os binários como anexos de uma GitHub Release na tag recém-criada, a partir de um arquivo `.goreleaser.yaml` na raiz do projeto (crie esse arquivo também, com build simples do `cmd/scribin`, archives e checksums).
- Evita loop: o job não deve rodar (ou deve pular a etapa de tag) se o commit do push já for um commit automático do próprio bot de release.
- Documenta no README.md uma seção "Releases" explicando que, pra gerar uma nova versão, basta mergear PRs pra main usando commits no padrão Conventional Commits (feat:, fix:, feat!: para breaking change etc.); o pipeline cuida da tag e do build.

Convenções: comentários em inglês nos arquivos de configuração (workflow e .goreleaser.yaml), nomes de job/step claros e diretos.
```

## Fase 8: Interface em terminal (TUI com Bubble Tea)

Pra não depender só de flags e logs no terminal, a camada de apresentação final é uma TUI com Bubble Tea: lista os vídeos encontrados, mostra barra de progresso por vídeo (e uma geral), e deixa acompanhar tudo sem abrir navegador nem empacotar app.

```
No projeto Go scribin, crie o pacote `internal/tui` usando Bubble Tea (`charm.land/bubbletea/v2`), Bubbles (`charm.land/bubbles/v2`, componente `progress`) e Lip Gloss (`charm.land/lipgloss/v2`) pra estilização, com:

- Um `Model` que recebe a lista de vídeos a processar (vinda do `internal/batch`) e mostra: uma lista com o nome de cada vídeo e seu status atual (pendente, extraindo áudio, transcrevendo, concluído, erro); uma barra de progresso individual por vídeo em processamento e uma barra de progresso geral (quantos de quantos já terminaram); um resumo final (sucessos, falhas, tempo total) quando tudo terminar.
- O `internal/batch` deve expor as atualizações de status via um channel (ex: `chan batch.ProgressEvent`, com campos como VideoName, Stage, Err) que o `tea.Program` consome via `tea.Cmd`/`tea.Msg`, sem acoplar o pacote `batch` ao Bubble Tea diretamente (ele só publica eventos no channel).
- Adicione uma flag `--tui` no `cmd/scribin/main.go` (fase 4): quando ativa, usa essa interface; quando ausente, mantém o comportamento atual de logs via `log.Printf` no stdout (sem travar quem preferir rodar em CI/scripts sem TUI).
- Trate o caso de terminal não interativo (ex: rodando dentro de outro script ou pipe), caindo de volta pro modo de log simples em vez de travar.

Convenções: comentários em inglês, nomes de pacote/função em inglês, reaproveitar o `internal/batch` sem duplicar a lógica de processamento (a TUI só consome eventos e mostra, não decide nada do pipeline).
```

