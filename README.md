# kareya-order-bridge

Bridge em Go entre a **API de integração da Brendi** (pedidos de marketplace) e a
**API do parceiro do Consumer** (sistema de gestão do restaurante).

O bridge é **sem banco de dados**. Ele faz polling na Brendi, traduz o pedido na
hora e responde ao Consumer. Quem persiste e deduplica os pedidos é o Consumer.

```
Consumer  --- GET  /v1/events:polling --->  bridge  --- GET /v1/events:polling --->  Brendi
          --- GET  /v1/orders/{id}   --->          --- GET /v1/orders/{id}   --->
          --- POST /v1/orders/{id}/status -->      --- POST confirm/cancel... --->
```

## Endpoints

Todos exigem token, exceto `/healthz`.

| Método | Rota | O que faz |
| --- | --- | --- |
| GET | `/healthz` | Health check (sem token), mostra a versão injetada no build. |
| GET | `/v1/events:polling` | Polling de pedidos novos. Traduz `CREATED` → `PLACED` (`PLC`). |
| GET | `/v1/events/{eventId}` | Consulta um evento pelo id. |
| GET | `/v1/orders/{orderId}` | Detalhes do pedido no formato do Consumer. |
| POST | `/v1/orders/{orderId}/status` | Recebe `{orderId, status, justification}` e repassa pra Brendi. |
| POST | `/v1/orders/{orderId}/status/{status}` | Idem, com status no path. |
| POST | `/v1/status` | Idem, com `orderId` no corpo. |
| POST | `/v1/status/{orderId}/{status}` | Idem, tudo no path (justificativa por query `?justification=`). |
| GET | `/v1/stores` | Lista a loja configurada. |

### Token

O Consumer manda o token em qualquer um destes formatos (o bridge aceita todos):

```
Authorization: Bearer <BRIDGE_TOKEN>
X-Api-Key: <BRIDGE_TOKEN>
?token=<BRIDGE_TOKEN>
?access_token=<BRIDGE_TOKEN>
```

Se o token não bater, o bridge responde `401` com
`{"statusCode":401,"reasonPhrase":"token invalido"}` e registra a tentativa no
log (ative `BRIDGE_LOG_HEADERS=true` para ver os headers e descobrir o formato
que a versão do Consumer do cliente usa).

### Status → Brendi

| Status do Consumer | Chamada na Brendi |
| --- | --- |
| `CONFIRMED` | `POST /v1/orders/{id}/confirm` |
| `CANCELLED` | `POST /v1/orders/{id}/requestCancellation` |
| `READY_FOR_PICKUP` | `POST /v1/orders/{id}/readyForPickup` |
| `DISPATCHED` | `POST /v1/orders/{id}/dispatch` |
| `CONCLUDED` | `POST /v1/orders/{id}/delivered` |

O cancelamento usa `code: INTERNAL_DIFFICULTIES_OF_THE_RESTAURANT` e
`mode: MANUAL`, porque a API da Brendi exige um código da lista fechada.

O manual do Consumer fala em `orderId`, `status` e `justification` como *path
parameters*, mas o exemplo manda tudo no corpo. O bridge aceita corpo, path e
query (`?status=CONFIRMED&justification=...`), então qualquer formato da tela do
Consumer funciona.

## Confirmação (ack) na Brendi

- Eventos que **não** são `CREATED` (confirmado, despachado, entregue...) são
  confirmados já no polling: o Consumer não consome esses eventos, ele mesmo
  gera os status.
- O evento `CREATED` só é confirmado **depois** que o Consumer busca
  `GET /v1/orders/{id}` (ou seja, depois que o pedido chegou no Consumer).
  Controlado por `BRIDGE_ACK_ON_DETAILS` (padrão `true`).

Consequência: se o Consumer nunca buscar os detalhes de um pedido, o evento
continua pendente na Brendi de propósito.

## Configuração

Copie `.env.example` para `.env.local` e preencha:

| Variável | Obrigatória | Descrição |
| --- | --- | --- |
| `BRENDI_CLIENT_ID` | sim | Client id da integração Brendi. |
| `BRENDI_CLIENT_SECRET` | sim | Client secret da Brendi. |
| `BRENDI_BASE_URL` | não | Padrão `https://api.brendi.com.br`. |
| `BRIDGE_TOKEN` | sim | Token que o Consumer vai enviar. Gere com `openssl rand -hex 32` (ou o comando PowerShell abaixo). |
| `PORT` | não | Porta HTTP (padrão `8080`). |
| `BRIDGE_HOST` | não | Interface de escuta (padrão `127.0.0.1`, só a própria máquina). Use `0.0.0.0` para expor na rede. |
| `BRIDGE_LOG_FILE` | não | Arquivo de log opcional (append, sem rotação). Útil no Agendador de Tarefas. |
| `BRIDGE_STORE_ID` | não | Id da loja em `/v1/stores` (padrão: client id da Brendi). |
| `BRIDGE_STORE_NAME` | não | Nome da loja em `/v1/stores`. |
| `BRIDGE_PRODUCT_MAP` | não | Caminho do JSON de de-para de produtos (relativo à pasta do `.env.local`). |
| `BRIDGE_ACK_ON_DETAILS` | não | `true` (padrão) confirma o evento ao entregar detalhes. |
| `BRIDGE_POLL_ALL_EVENTS` | não | `true` reenvia qualquer evento como `PLACED` (só use se estiver perdendo pedido). |
| `BRIDGE_LOG_HEADERS` | não | `true` loga headers de requisições sem token. |

### De-para de produtos (`externalCode`)

Pedidos de marketplace (99Food) vêm com `externalCode` vazio. O Consumer precisa
do código PDV do produto. Crie um JSON assim e aponte `BRIDGE_PRODUCT_MAP`:

```json
{
  "byExternalCode": {
    "CODIGO-QUE-A-BRENDI-MANDA": "112"
  },
  "byName": {
    "Donburi katsudon": "112",
    "Guioza suíno (6 un)": "205"
  }
}
```

A busca tenta `byExternalCode` primeiro e depois `byName` (nome exato do item na
Brendi). Sem mapa, o bridge devolve o `externalCode` original; se ele vier
vazio, o Consumer provavelmente não consegue casar o item com o PDV.

## Rodar local

```bash
cp .env.example .env.local
# edite .env.local (chmod 600 .env.local)
go run ./cmd/bridge
```

Teste rápido:

```bash
curl -H "Authorization: Bearer $BRIDGE_TOKEN" http://localhost:8080/v1/stores
curl -H "Authorization: Bearer $BRIDGE_TOKEN" http://localhost:8080/v1/events:polling
```

Ferramenta separada para inspecionar a Brendi sem o bridge:

```bash
go run ./cmd/brendi-poll        # baixa eventos/pedidos, não confirma
go run ./cmd/brendi-poll -ack   # confirma depois de salvar
```

## Registrar no Consumer

No Consumer do restaurante (mesma máquina Windows do bridge):

1. `APPS → Pedidos Online → API do parceiro`.
2. Preencha token (o mesmo `BRIDGE_TOKEN`), CNPJ, razão social e URL da loja.
3. Cadastre as URLs usando **`127.0.0.1`**, não `localhost` (no Windows o
   `localhost` pode cair no IPv6 `::1`, e o bridge escuta em IPv4):

| Campo do Consumer | URL |
| --- | --- |
| URL de polling | `http://127.0.0.1:8080/v1/events:polling` |
| URL de eventos | `http://127.0.0.1:8080/v1/events` (alias do polling) |
| URL de alteração de status | `http://127.0.0.1:8080/v1/orders/{orderId}/status` |
| URL de listagem de lojas | `http://127.0.0.1:8080/v1/stores` |
| URL de detalhes do pedido | `http://127.0.0.1:8080/v1/orders/{orderId}` |

4. Ative `Principal → Fila de Pedidos`.

> Se a tela do Consumer recusar URL `http://` ou exigir `https://`, me avise:
> aí precisamos colocar um TLS local na frente do bridge (Caddy, por exemplo).
> Não há documentação dizendo se o campo exige HTTPS.

> Os caminhos de **eventos** e **listagem de lojas** seguem o formato do guia de
> integração, mas não há documentação pública detalhada desses dois endpoints.
> Se a tela do Consumer do cliente pedir outro caminho/JSON, ajuste em
> `internal/bridge/server.go` (as rotas ficam todas em `Handler`).

## Instalar no Windows (junto do Consumer)

O bridge escuta só em `127.0.0.1`, então o Consumer conversa com ele pela própria
máquina. Não precisa de IP público, domínio nem TLS.

### 1. Build

De uma máquina Linux (ou do próprio Windows, com Go instalado):

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bridge.exe ./cmd/bridge
```

### 2. Pasta na máquina do Consumer

Copie para algo como `C:\kareya\bridge\`:

```
bridge.exe
.env.local
products.json   (opcional)
```

O bridge procura o `.env.local` **na mesma pasta do executável** (e só cai para o
diretório atual se não achar), então funciona mesmo quando o serviço inicia com
outro working directory. Caminhos relativos de `BRIDGE_PRODUCT_MAP` são
resolvidos a partir dessa pasta. O `.env.local` pode ter CRLF e BOM (Notepad) que
o bridge lida normalmente.

Gere o token no PowerShell:

```powershell
$b = New-Object byte[] 32
[Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b)
($b | ForEach-Object { $_.ToString('x2') }) -join ''
```

### 3. Rodar como serviço (NSSM)

O jeito mais simples de manter rodando no boot e reiniciar sozinho. Baixe o
[NSSM](https://nssm.cc/download), extraia e rode em um PowerShell **como
administrador**:

```powershell
nssm install KareyaBridge C:\kareya\bridge\bridge.exe
nssm set KareyaBridge AppDirectory C:\kareya\bridge
nssm set KareyaBridge AppStdout C:\kareya\bridge\bridge.log
nssm set KareyaBridge AppStderr C:\kareya\bridge\bridge.log
nssm set KareyaBridge AppRotateFiles 1
nssm set KareyaBridge AppRotateOnline 1
nssm set KareyaBridge AppRotateBytes 10485760
nssm set KareyaBridge Start SERVICE_AUTO_START
nssm start KareyaBridge
```

Se usar o redirecionamento de log do NSSM, **não** defina `BRIDGE_LOG_FILE`
(senão o log sai duplicado).

Alternativa sem NSSM, usando o Agendador de Tarefas e `BRIDGE_LOG_FILE`:

```powershell
schtasks /Create /TN "KareyaBridge" /TR "C:\kareya\bridge\bridge.exe" /SC ONSTART /RU SYSTEM /RL HIGHEST /F
```

### 4. Conferir

```powershell
curl.exe http://127.0.0.1:8080/healthz
curl.exe -H "Authorization: Bearer SEU_TOKEN" http://127.0.0.1:8080/v1/stores
```

### 5. Cuidados na máquina

- **Firewall**: nenhuma regra de entrada é necessária enquanto escutar em
  `127.0.0.1`. Só precisa de saída HTTPS (443) para `api.brendi.com.br`.
- **Antivírus**: o exe não é assinado; se for bloqueado, adicione a pasta
  `C:\kareya\bridge` nas exclusões.
- **Energia**: desligue suspensão/hibernação para o polling não parar
  (`powercfg /change standby-timeout-ac 0`).
- **Relógio**: mantenha o fuso e o horário do Windows corretos; os eventos usam
  data/hora.

## Alternativa: VPS Linux

Se o bridge não ficar na mesma máquina do Consumer, aí sim precisa de endereço
público com HTTPS. Build estático:

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bridge ./cmd/bridge
```

Exemplo de unit systemd (`/etc/systemd/system/kareya-bridge.service`):

```ini
[Unit]
Description=Kareya order bridge (Brendi -> Consumer)
After=network-online.target
Wants=network-online.target

[Service]
User=kareya
WorkingDirectory=/opt/kareya-order-bridge
EnvironmentFile=/opt/kareya-order-bridge/.env.local
ExecStart=/opt/kareya-order-bridge/bridge
Restart=always
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

Nesse caso, use `BRIDGE_HOST=0.0.0.0`, coloque um reverse proxy (Caddy/Nginx) na
frente para terminar o TLS e cadastre no Consumer as URLs públicas
`https://bridge.exemplo.com/...`.

## Releases (GitHub Actions)

O repositório tem dois workflows:

- **ci** (`push` na `main` e pull requests): `gofmt`, `go vet`, `go test -race`
  e um build cruzado para Windows.
- **release** (`push` de tag `v*`): o GoReleaser compila e publica os binários
  na release do GitHub.

Para lançar uma versão:

```bash
git tag -a v0.0.0 -m "versao inicial"
git push origin v0.0.0
```

Artefatos gerados: `linux/amd64`, `linux/arm64`, `windows/amd64`,
`windows/arm64`, `darwin/amd64`, `darwin/arm64` e `checksums.txt`. No Windows o
pacote é `.zip` com `bridge.exe`; nos demais, `.tar.gz`. A versão da tag aparece
no `/healthz` e no log de inicialização.

Para validar o GoReleaser localmente antes de taguear:

```bash
goreleaser check
goreleaser release --snapshot --clean
```

## Limitações conhecidas

- Não há endpoint de lista de pedidos na Brendi: só dá pra buscar por id. Se o
  bridge ficar fora do ar e o evento `CREATED` expirar, o pedido não é
  recuperado.
- `customer.phone.localizer` e `delivery.pickupCode` não vêm no pedido da Brendi.
  O bridge envia os campos vazios (`""`) para manter o contrato do Consumer;
  se o Consumer exigir um código de verdade, precisamos gerar/mapear um.
- O endpoint de **envio dos detalhes do pedido** (o Consumer mandando detalhes
  para o parceiro, usado no fluxo inverso) não é implementado: nesse sentido
  (Brendi → Consumer) os detalhes já vêm da Brendi.
- `GET /v1/stores` devolve só a loja configurada (a Brendi não expõe lojas). Esse
  endpoint não aparece no manual, mas algumas versões da tela pedem a URL.
- `orderExternalCode` no `confirm` usa o `displayId` do pedido; se a Brendi
  recusar, ajuste em `internal/brendi/client.go`.
- Sem sandbox do Consumer: teste direto na instalação do cliente.
