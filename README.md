# scribin

Utilitário em Go, 100% local, que varre uma pasta de vídeos já gravados,
extrai o áudio de cada um (via ffmpeg) e gera a transcrição (texto e SRT)
usando o [whisper.cpp](https://github.com/ggerganov/whisper.cpp) como motor,
sem nenhuma dependência de API em nuvem.

## Pré-requisitos

- Go 1.22 ou mais recente
- [ffmpeg](https://ffmpeg.org/) no PATH
- cmake e um compilador C/C++ (gcc ou clang), para compilar o whisper.cpp
- git

Com os pré-requisitos instalados, rode o script de setup a partir da raiz do
projeto para clonar e compilar o whisper.cpp e baixar o modelo `small`:

```sh
scripts/setup.sh
```

O script é idempotente (pode ser rodado de novo sem duplicar o clone ou o
download) e, ao final, roda uma transcrição de teste pra confirmar que a
cadeia ffmpeg + whisper.cpp está funcionando. O clone do whisper.cpp fica em
`third_party/whisper.cpp/` e os modelos baixados em `models/` — ambos fora
do controle de versão.

> Nota: em alguns ambientes Windows com o recurso **Smart App Control**
> ativado, o toolchain de compilação (MinGW/GCC) pode ser bloqueado. Nesse
> caso, rode o script dentro do WSL (Windows Subsystem for Linux).

## Compilando

```sh
go build -o scribin ./cmd/scribin
```

## Uso

```sh
./scribin \
  --input-dir ./videos \
  --output-dir ./transcricoes \
  --model third_party/whisper.cpp/models/ggml-small.bin \
  --whisper-bin third_party/whisper.cpp/build/bin/whisper-cli \
  --lang pt
```

Para cada vídeo encontrado recursivamente em `--input-dir` (extensões `.mp4`,
`.mkv`, `.mov`, `.avi`), o comando extrai o áudio, transcreve e grava
`<nome-do-video>.txt` (texto corrido) e `<nome-do-video>.srt` (com
timestamps) em `--output-dir`. Vídeos que já têm um `.txt` correspondente em
`--output-dir` são pulados, então rodar o comando de novo sobre a mesma
pasta só processa o que for novo.

### Flags disponíveis

| Flag                   | Obrigatória | Default                  | Descrição                                                                 |
|-------------------------|:-----------:|---------------------------|----------------------------------------------------------------------------|
| `--input-dir`            | sim         | —                          | Diretório com os vídeos a transcrever (varredura recursiva)               |
| `--output-dir`           | sim         | —                          | Diretório onde `.txt`/`.srt` são escritos                                  |
| `--model`                | sim         | —                          | Caminho do modelo ggml (ex: `ggml-small.bin`)                              |
| `--whisper-bin`          | sim         | —                          | Caminho do binário `whisper-cli`                                           |
| `--lang`                 | não         | `pt`                       | Idioma falado nos vídeos (ex: `pt`, `en`, ou `auto` para auto-detecção)   |
| `--workers`              | não         | metade das CPUs disponíveis| Quantos vídeos processar em paralelo                                       |
| `--per-video-timeout`    | não         | `30m`                      | Timeout individual por vídeo (extração + transcrição); evita que um vídeo corrompido trave o lote inteiro |
| `--dry-run`              | não         | `false`                    | Só lista os vídeos que seriam processados, sem chamar ffmpeg/whisper      |

O comando continua processando os demais vídeos mesmo se um falhar
individualmente (o erro é logado e o lote segue), mas termina com código de
saída diferente de zero se houve qualquer falha — útil para detectar
problemas em scripts/CI.

## Testes

```sh
go test ./...
```

O teste de integração de ponta a ponta em `cmd/scribin` usa o vídeo curto em
`testdata/sample.mp4` e só roda se `ffmpeg`, o binário `whisper-cli` e um
modelo ggml já estiverem disponíveis (ou seja, depois de rodar
`scripts/setup.sh`); caso contrário, ele é pulado automaticamente.
