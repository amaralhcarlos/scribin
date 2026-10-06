# scribin

Utilitário Go, 100% local, que varre uma pasta de vídeos já gravados, extrai o áudio de cada um (via ffmpeg) e gera a transcrição (texto e SRT) usando o whisper.cpp como motor, sem nenhuma dependência de API em nuvem. O plano completo, fase a fase, com os prompts prontos pra cada etapa, está em `PLAN.md` nesta mesma pasta.

## Convenções de código

- Comentários no código em inglês.
- Logs em Go com `log.Printf`/`fmt.Printf`. Não customizar handlers de `slog`.
- Preferir soluções reutilizáveis: pacotes/funções genéricas em vez de código duplicado entre as diferentes partes do pipeline (extract, transcribe, batch, store, tui).
- Nomes de pacotes e identificadores em inglês.
- Módulo Go e binário CLI chamam-se `scribin` (`cmd/scribin`).

## Estrutura de pastas

```
scribin/
  cmd/scribin/               # CLI entrypoint
  internal/extract/         # wrapper do ffmpeg
  internal/transcribe/      # wrapper do whisper-cli
  internal/batch/           # orquestração/worker pool
  internal/store/           # persistência opcional no Postgres
  internal/tui/             # interface em terminal (Bubble Tea)
  third_party/whisper.cpp/  # clone compilado localmente
  models/                   # modelos ggml baixados
```

## Fluxo de branches

O repositório trabalha com duas branches fixas: `main` e `develop`. Todo desenvolvimento novo (features, correções etc.) parte de `develop`, nunca direto de `main`.

- `develop`: branch de integração, recebe os PRs de feature/fix do dia a dia.
- `main`: branch estável; só recebe merge de `develop` quando o conteúdo acumulado está pronto pra virar uma nova versão.
- É o merge de `develop` pra `main` que dispara o pipeline de release (GitHub Actions: gera a tag semver via Conventional Commits e publica os binários com GoReleaser).

Commits devem seguir Conventional Commits (`feat:`, `fix:`, `feat!:`/`fix!:` para breaking change) porque é isso que o pipeline de release usa para decidir a próxima versão.

## Como usar este repositório com o Claude Code

Implemente uma fase do `PLAN.md` por vez, não todas de uma vez: cole o prompt da fase atual, revise o diff gerado, rode/teste o que fizer sentido, e só então comite na `develop` antes de avançar pra próxima fase. Para as fases mais densas em decisão de arquitetura (orquestração em lote e TUI), vale entrar em modo de planejamento antes de pedir a implementação.
