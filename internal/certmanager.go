package internal

import (
	"context"
	acmev1 "github.com/cert-manager/cert-manager/pkg/apis/acme/v1"
	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c clients) GetCertificateRequestForCertificate(cert *certv1.Certificate) (*certv1.CertificateRequest, error) {
	// Find certificate request matching the cert
	crslist, err := c.CertManagerClient().CertificateRequests(cert.Namespace).List(context.Background(), v1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var found *certv1.CertificateRequest
	for i, _ := range crslist.Items {
		crs := crslist.Items[i]
		if len(crs.OwnerReferences) == 0 {
			continue
		}

		if crs.OwnerReferences[0].Kind != "Certificate" {
			continue
		}

		if crs.OwnerReferences[0].Name == cert.Name && crs.OwnerReferences[0].UID == cert.UID {
			if found == nil || crs.ObjectMeta.CreationTimestamp.After(found.ObjectMeta.CreationTimestamp.Time) {
				found = &crs
			}
		}
	}
	return found, nil
}

func (c clients) GetOrderForCertificateRequest(crs *certv1.CertificateRequest) (*acmev1.Order, error) {
	orderList, err := c.AcmeClient().Orders(crs.Namespace).List(context.Background(), v1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var found *acmev1.Order
	for i, _ := range orderList.Items {
		order := orderList.Items[i]
		if len(order.OwnerReferences) == 0 {
			continue
		}

		if order.OwnerReferences[0].Kind != "CertificateRequest" {
			continue
		}

		if order.OwnerReferences[0].Name == crs.Name && order.OwnerReferences[0].UID == crs.UID {
			if found == nil || order.ObjectMeta.CreationTimestamp.After(found.CreationTimestamp.Time) {
				found = &order
			}
		}
	}
	return found, nil
}

func (c clients) GetChallengesForOrder(order *acmev1.Order) ([]*acmev1.Challenge, error) {
	challengeList, err := c.AcmeClient().Challenges(order.Namespace).List(context.Background(), v1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var challenges []*acmev1.Challenge
	for i, _ := range challengeList.Items {
		chall := challengeList.Items[i]
		if len(chall.OwnerReferences) == 0 {
			continue
		}

		if chall.OwnerReferences[0].Kind != "Order" {
			continue
		}

		if chall.OwnerReferences[0].Name == order.Name && chall.OwnerReferences[0].UID == order.UID {
			challenges = append(challenges, &chall)
		}
	}
	return challenges, nil
}
