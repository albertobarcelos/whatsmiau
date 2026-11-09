# Fork Nexsyn — videochamada 1:1

Repositório: https://github.com/albertobarcelos/whatsmiau
Base: WhatsMiau `v1.7.0` (`fa4e614`)
Licença: Elastic License 2.0 (uso interno; ver `LICENSE` e `NOTICE`)

## Por que este fork

O WhatsMiau oficial já usa `meowcaller`, que transporta H.264 Annex-B.
A API pública só abria PCM (voz). Este fork espelha o bridge de áudio para vídeo.

Não inventamos o protocolo VoIP do WhatsApp. Não atualizamos o meowcaller para a
linha `hypermeow` (quebra o `whatsmeow` que o Miau já usa).

## Camadas

1. **Este fork (Go)** — `POST /calls` com `video`, `GET /calls/{id}/video` (WS H.264),
   `POST /calls/{id}/video` (câmera / upgrade).
2. **Bridge Node (Chatwoot)** — proxy REST/SSE/WS sem expor a API key no browser.
3. **Widget no painel** — WebCodecs no Chrome/Edge (o servidor não decodifica pixel).

## API nova (autenticada com `apikey`, igual ao áudio)

| Método | Caminho | Função |
| --- | --- | --- |
| POST | `/v1/instance/{instance}/calls` | `{"number":"...","video":true}` liga com vídeo |
| GET | `/v1/instance/{instance}/calls/{callID}/video` | WebSocket: 1 mensagem binária = 1 access unit Annex-B |
| POST | `/v1/instance/{instance}/calls/{callID}/video` | `{"enabled":true}` / `{"accept":true}` / `{"orientation":0}` |

No WS, frames são binários. Texto JSON `{"type":"keyframe"}` pede IDR ao encoder do browser.
`{"type":"state","peerVideo":true,"orientation":0}` reflete a câmera do par.

Áudio continua em `/calls/{id}/audio` (PCM 16 kHz). Os dois sockets abrem juntos.

## Status

- Voz: validada no painel Chatwoot.
- Vídeo: experimental (o próprio meowcaller marca o path de mídia como não validado).
  MVP: 1:1, Chrome/Edge. Grupo e tela ficam depois.

## Deploy

Imagem de produção continua sendo a oficial até este branch ser construído e
testado contra um celular real. Trocar `impedr029/whatsmiau:v1.7.0` só depois
do teste de vídeo.
