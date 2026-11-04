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
| GET | `/healthz` | Health check (sem token). |
| GET | `/v1/events:polling` | Polling de pedidos novos. Traduz `CREATED` → `PLACED` (`PLC`). |
| GET | `/v1/events/{eventId}` | Consulta um evento pelo id. |
| GET | `/v1/orders/{orderId}` | Detalhes do pedido no formato do Consumer. |
| POST | `/v1/orders/{orderId}/status` | Recebe `{orderId, status, justification}` e repassa pra Brendi. |
| POST | `/v1/status` | Igual acima, com `orderId` no corpo. |
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
| `BRIDGE_TOKEN` | sim | Token que o Consumer vai enviar. Gere com `openssl rand -hex 32`. |
| `PORT` | não | Porta HTTP (padrão `8080`). |
| `BRIDGE_STORE_ID` | não | Id da loja em `/v1/stores` (padrão: client id da Brendi). |
| `BRIDGE_STORE_NAME` | não | Nome da loja em `/v1/stores`. |
| `BRIDGE_PRODUCT_MAP` | não | Caminho do JSON de de-para de produtos. |
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

No Consumer do restaurante:

1. `APPS → Pedidos Online → API do parceiro`.
2. Preencha token (o mesmo `BRIDGE_TOKEN`), CNPJ, razão social e URL da loja.
3. Cadastre as URLs (troque `https://bridge.exemplo.com` pelo endereço público
   com HTTPS):

| Campo do Consumer | URL |
| --- | --- |
| URL de polling | `https://bridge.exemplo.com/v1/events:polling` |
| URL de eventos | `https://bridge.exemplo.com/v1/events` (alias do polling) |
| URL de alteração de status | `https://bridge.exemplo.com/v1/orders/{orderId}/status` |
| URL de listagem de lojas | `https://bridge.exemplo.com/v1/stores` |
| URL de detalhes do pedido | `https://bridge.exemplo.com/v1/orders/{orderId}` |

4. Ative `Principal → Fila de Pedidos`.

> Os caminhos de **eventos** e **listagem de lojas** seguem o formato do guia de
> integração, mas não há documentação pública detalhada desses dois endpoints.
> Se a tela do Consumer do cliente pedir outro caminho/JSON, ajuste em
> `internal/bridge/server.go` (as rotas ficam todas em `Handler`).

## Deploy na VPS

Build estático e cópia do binário:

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

Coloque um reverse proxy (Caddy/Nginx) na frente para terminar o TLS. Aponte o
`BRIDGE_TOKEN` só para o Consumer; o token da Brendi fica apenas no servidor.

## Limitações conhecidas

- Não há endpoint de lista de pedidos na Brendi: só dá pra buscar por id. Se o
  bridge ficar fora do ar e o evento `CREATED` expirar, o pedido não é
  recuperado.
- `customer.phone.localizer` (código de entrega) não vem no pedido da Brendi;
  o campo fica de fora.
- `GET /v1/stores` devolve só a loja configurada (a Brendi não expõe lojas).
- `orderExternalCode` no `confirm` usa o `displayId` do pedido; se a Brendi
  recusar, ajuste em `internal/brendi/client.go`.
- Sem sandbox do Consumer: teste direto na instalação do cliente.
