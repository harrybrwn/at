package atp

import (
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/whyrusleeping/go-did"
)

func ConvertDidDoc(doc *did.Document) *identity.DIDDocument {
	res := identity.DIDDocument{
		DID:         syntax.DID(doc.ID.String()),
		AlsoKnownAs: doc.AlsoKnownAs,
	}
	for i := range doc.VerificationMethod {
		vm := &doc.VerificationMethod[i]
		method := identity.DocVerificationMethod{
			ID:         vm.ID,
			Type:       vm.Type,
			Controller: vm.Controller,
		}
		if vm.PublicKeyMultibase != nil {
			method.PublicKeyMultibase = *vm.PublicKeyMultibase
		}
		res.VerificationMethod = append(res.VerificationMethod, method)
	}
	for _, svc := range doc.Service {
		service := identity.DocService{
			ID:              svc.ID.String(),
			Type:            svc.Type,
			ServiceEndpoint: svc.ServiceEndpoint,
		}
		res.Service = append(res.Service, service)
	}
	return &res
}

func IdentityToDoc(i *identity.Identity) (*did.Document, error) {
	var err error
	doc := did.Document{
		AlsoKnownAs: i.AlsoKnownAs,
	}
	doc.ID, err = did.ParseDID(i.DID.String())
	if err != nil {
		return nil, err
	}
	return &doc, nil
}
