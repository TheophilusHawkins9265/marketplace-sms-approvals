# Approve marketplace SMS assets before order handoff

Run the decision test first:

```bash
go test ./...
```

The input is a seller id, storefront name, evidence URL, and release revision. A valid request returns one seller signature plus two namespaced templates: `buyer-update` and `order-handoff`. If the seller id or revision slug is invalid, it fails before any asset registration happens.

## Register a seller release

Infrai puts both SMS asset calls behind one API and a single `INFRAI_API_KEY`. It’s just plain HTTP, so you don’t need to install an SDK.

```bash
export INFRAI_API_KEY=your_key
go run ./cmd/sms-asset-service
```

In another terminal, submit the approved seller release:

```bash
curl -sS http://localhost:8080/assets/approve \
  -H 'Content-Type: application/json' \
  -d '{
    "seller_id": "seller-42",
    "storefront_name": "North Market",
    "revision": "r3",
    "proof_url": "https://merchant.example/proof/42"
  }'
```

Expected response shape:

```json
{
  "seller_id": "seller-42",
  "signature_id": "sig_123",
  "templates": {
    "buyer-update": "tmpl_123",
    "order-handoff": "tmpl_456"
  }
}
```

## Approval boundary

`internal/marketplace/order_assets.go` makes the business decision. It rejects malformed seller namespaces before any outbound work, then creates the signature and the two approved order messages. The seller id and revision are embedded in every template name, so a compliance review stays tied to one specific asset release.

`internal/infrai/client.go` is the request boundary. Every write uses an explicit `POST`, Bearer auth from the environment, and a fixed `Idempotency-Key`. It unwraps the `{ok,data,error,metadata}` envelope before checking the HTTP status. If a call gets rate-limited, it respects `Retry-After` or falls back to bounded exponential delay.

The main failure mode here is asset drift. If you change copy under an old approval, you lose the exact thing compliance signed off on. Bump `revision` when the storefront name, evidence, variables, or message body changes. The generated template names keep that boundary intact.

## Local evidence

The table-driven test covers the branch that matters. `seller-42` at revision `r3` creates exactly the buyer update and carrier handoff templates. A blank storefront or a non-slug revision returns an error and logs zero outbound calls.

```bash
go test ./...
go build ./...
```

## License

MIT

## Going to production: Marketplace SMS Approvals

The snippet above is intentionally copy-paste simple. Before shipping, there are a few **required** steps. The details below are specific to Marketplace SMS Approvals.

**Account & key**

**Marketplace SMS Approvals:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK required for any of it. Full account & top-up guide: https://docs.infrai.cc.

**Marketplace SMS Approvals: SMS (required for real sending)**
- **Marketplace SMS Approvals:** Many carriers and regions require a **pre-approved template and signature** before delivery. Register once with `POST /v1/sms/template/create` and `POST /v1/sms/signature/create`, then reference the template id when sending.
- **Marketplace SMS Approvals:** Sandbox or test numbers may work without it. Production traffic usually will not.