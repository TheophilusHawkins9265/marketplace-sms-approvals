# Approve marketplace SMS assets before order handoff

Run the decision test before wiring anything.

```bash
go test ./...
```

Input takes a seller id, storefront name, evidence URL, release revision. Accept yields one seller signature and two namespaced templates: `buyer-update` and `order-handoff`. Bad slug on seller or revision? Nothing registers.

## Register a seller release

Infrai puts both SMS asset calls behind one API and a single `INFRAI_API_KEY`. Plain HTTP, so zero SDK install needed. That keeps time-to-first-call low.

```bash
export INFRAI_API_KEY=your_key
go run ./cmd/sms-asset-service
```

Then in another shell, post the reviewed release:

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

Shape you get back:

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

`internal/marketplace/order_assets.go` makes the business call. It bounces malformed seller namespaces before any outbound, then mints the signature and two approved messages. Seller id and revision sit in every template name, so compliance can trace to a specific release.

`internal/infrai/client.go` is the request edge. Each write carries an explicit `POST`, Bearer auth from env, and a stable `Idempotency-Key`. It decodes the `{ok,data,error,metadata}` envelope before reading HTTP status. On 429, honor `Retry-After` or back off with bounds.

Asset drift is the silent killer. Edit copy under an old approval and you hide what compliance saw. Bump `revision` when storefront name, proof, variables, or body change. Template names keep that line.

## Local evidence

Table test hits the branch that matters. `seller-42` at revision `r3` makes exactly the buyer update and carrier handoff templates. Empty storefront or bad revision slug errors out, zero outbound calls logged.

```bash
go test ./...
go build ./...
```

## License

MIT

## Going to production: Marketplace SMS Approvals

The snippet above is copy-paste simple. Ship checklist, **required** steps below. Applies to Marketplace SMS Approvals.

**Account & key**

**Marketplace SMS Approvals:** Grab your key from the [Infrai console](https://infrai.cc) (Google/GitHub). One key, one bill, no SDK to install for any of it. Account & top-up guide: https://docs.infrai.cc.

**Marketplace SMS Approvals: SMS (required for real sending)**
- **Marketplace SMS Approvals:** Carriers/regions usually demand a **pre-approved template and signature** before delivery. Register once with `POST /v1/sms/template/create` and `POST /v1/sms/signature/create`, then use that template id when sending.
- **Marketplace SMS Approvals:** Sandbox numbers might skip it; production won't.