package marketplace

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/example/marketplace-sms-approvals/internal/infrai"
)

type recordingCreator struct {
	signatures []infrai.SignatureRequest
	templates  []infrai.TemplateRequest
	keys       []string
	deleted    []string
	failSecond bool
}

func (f *recordingCreator) CreateSignature(_ context.Context, request infrai.SignatureRequest, key string) (infrai.Signature, error) {
	f.signatures = append(f.signatures, request)
	f.keys = append(f.keys, key)
	return infrai.Signature{SignatureID: "sig_market"}, nil
}

func (f *recordingCreator) CreateTemplate(_ context.Context, request infrai.TemplateRequest, key string) (infrai.Template, error) {
	if f.failSecond && len(f.templates) == 1 {
		return infrai.Template{}, errors.New("template rejected")
	}
	f.templates = append(f.templates, request)
	f.keys = append(f.keys, key)
	return infrai.Template{TemplateID: "tmpl_" + request.Name}, nil
}

func (f *recordingCreator) DeleteSignature(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *recordingCreator) DeleteTemplate(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func TestApproveCleansUpPartialAssets(t *testing.T) {
	fake := &recordingCreator{failSecond: true}
	_, err := NewProvisioner(fake).Approve(context.Background(), AssetRequest{
		SellerID: "seller-42", StorefrontName: "North Market", Revision: "r3",
	})
	if err == nil {
		t.Fatal("expected template failure")
	}
	want := []string{"tmpl_seller-42-order-r3-buyer-update", "sig_market"}
	if !reflect.DeepEqual(fake.deleted, want) {
		t.Fatalf("deleted = %v, want %v", fake.deleted, want)
	}
}

func TestApproveOrderAssets(t *testing.T) {
	tests := []struct {
		name          string
		input         AssetRequest
		wantErr       bool
		wantTemplates []string
	}{
		{
			name:          "approved seller gets buyer and handoff assets",
			input:         AssetRequest{SellerID: "seller-42", StorefrontName: "North Market", Revision: "r3", ProofURL: "https://merchant.example/proof/42"},
			wantTemplates: []string{"seller-42-order-r3-buyer-update", "seller-42-order-r3-order-handoff"},
		},
		{name: "blank storefront is denied before registration", input: AssetRequest{SellerID: "seller-42", Revision: "r3"}, wantErr: true},
		{name: "unnamespaced revision is denied", input: AssetRequest{SellerID: "seller-42", StorefrontName: "North Market", Revision: "Release 3"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &recordingCreator{}
			result, err := NewProvisioner(fake).Approve(context.Background(), tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Approve() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if len(fake.signatures)+len(fake.templates) != 0 {
					t.Fatal("denied request reached asset registration")
				}
				return
			}
			gotNames := []string{fake.templates[0].Name, fake.templates[1].Name}
			if !reflect.DeepEqual(gotNames, tt.wantTemplates) {
				t.Fatalf("template names = %v, want %v", gotNames, tt.wantTemplates)
			}
			if result.SignatureID != "sig_market" || len(result.Templates) != 2 {
				t.Fatalf("unexpected approved assets: %+v", result)
			}
			if len(fake.keys) != 3 || fake.keys[0] == fake.keys[1] || fake.keys[1] == fake.keys[2] {
				t.Fatalf("asset keys are not stable and distinct: %v", fake.keys)
			}
		})
	}
}
