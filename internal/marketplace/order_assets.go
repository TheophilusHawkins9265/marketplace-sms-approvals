package marketplace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/example/marketplace-sms-approvals/internal/infrai"
)

var assetPart = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,39}$`)

type AssetRequest struct {
	SellerID       string `json:"seller_id"`
	StorefrontName string `json:"storefront_name"`
	Revision       string `json:"revision"`
	ProofURL       string `json:"proof_url"`
}

type ApprovedAssets struct {
	SellerID    string            `json:"seller_id"`
	SignatureID string            `json:"signature_id"`
	Templates   map[string]string `json:"templates"`
}

type AssetCreator interface {
	CreateSignature(context.Context, infrai.SignatureRequest, string) (infrai.Signature, error)
	CreateTemplate(context.Context, infrai.TemplateRequest, string) (infrai.Template, error)
	DeleteSignature(context.Context, string) error
	DeleteTemplate(context.Context, string) error
}

type Provisioner struct {
	client AssetCreator
}

func NewProvisioner(client AssetCreator) *Provisioner {
	return &Provisioner{client: client}
}

func (p *Provisioner) Approve(ctx context.Context, input AssetRequest) (result ApprovedAssets, err error) {
	if !assetPart.MatchString(input.SellerID) || !assetPart.MatchString(input.Revision) {
		return ApprovedAssets{}, errors.New("seller_id and revision must be lowercase slugs")
	}
	if strings.TrimSpace(input.StorefrontName) == "" {
		return ApprovedAssets{}, errors.New("storefront_name is required")
	}

	namespace := input.SellerID + "-order-" + input.Revision
	signature, err := p.client.CreateSignature(ctx, infrai.SignatureRequest{
		Name: input.StorefrontName, Type: "company", ProofURL: input.ProofURL,
		Remark: "Marketplace seller order updates",
	}, stableKey(namespace, "signature"))
	if err != nil {
		return ApprovedAssets{}, fmt.Errorf("create seller signature: %w", err)
	}
	createdTemplates := []string{}
	defer func() {
		if err == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		for i := len(createdTemplates) - 1; i >= 0; i-- {
			if cleanupErr := p.client.DeleteTemplate(cleanupCtx, createdTemplates[i]); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("delete template %s: %w", createdTemplates[i], cleanupErr))
			}
		}
		if cleanupErr := p.client.DeleteSignature(cleanupCtx, signature.SignatureID); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("delete signature %s: %w", signature.SignatureID, cleanupErr))
		}
	}()

	templates := []struct {
		stage string
		body  string
		vars  []string
	}{
		{stage: "buyer-update", body: "Order {{order_id}} from {{seller_name}} is confirmed. Track: {{tracking_url}}", vars: []string{"order_id", "seller_name", "tracking_url"}},
		{stage: "order-handoff", body: "Order {{order_id}} was handed to {{carrier}}. Tracking: {{tracking_url}}", vars: []string{"order_id", "carrier", "tracking_url"}},
	}

	approved := ApprovedAssets{SellerID: input.SellerID, SignatureID: signature.SignatureID, Templates: make(map[string]string, len(templates))}
	for _, item := range templates {
		name := namespace + "-" + item.stage
		created, err := p.client.CreateTemplate(ctx, infrai.TemplateRequest{
			Name: name, Body: item.body, Locale: "en", Variables: item.vars, MessageType: "transactional",
		}, stableKey(namespace, item.stage))
		if err != nil {
			return ApprovedAssets{}, fmt.Errorf("create %s template: %w", item.stage, err)
		}
		createdTemplates = append(createdTemplates, created.TemplateID)
		approved.Templates[item.stage] = created.TemplateID
	}
	return approved, nil
}

func stableKey(namespace, asset string) string {
	sum := sha256.Sum256([]byte(namespace + ":" + asset))
	return "marketplace-sms-" + hex.EncodeToString(sum[:12])
}
